package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func handleIsLoggedIn() {
	log.Infof("Checking if logged in...")
	log.Infof("Logged in: %t", cli.IsLoggedIn())
}

func handleCheckUser(args []string) {
	log.Infof("Checking users: %v", args)
	if len(args) < 1 {
		log.Errorf("Usage: checkuser <phone numbers...>")
		return
	}

	resp, err := cli.IsOnWhatsApp(args)
	if err != nil {
		log.Errorf("Failed to check if users are on WhatsApp: %v", err)
		return
	}

	for _, item := range resp {
		logMessage := fmt.Sprintf("%s: on WhatsApp: %t, JID: %s", item.Query, item.IsIn, item.JID)

		if item.VerifiedName != nil {
			logMessage += fmt.Sprintf(", business name: %s", item.VerifiedName.Details.GetVerifiedName())
		}

		log.Infof(logMessage)

		// Send response to websocket
		if wsConn != nil {
			wsConn.WriteJSON(item)
		}
	}
}

func newHandleCheckUser(args []string) (response []types.IsOnWhatsAppResponse) {
	log.Infof("Checking users: %v", args)
	if len(args) < 1 {
		log.Errorf("Usage: checkuser <phone numbers...>")
		return nil
	}

	resp, err := cli.IsOnWhatsApp(args)
	if err != nil {
		log.Errorf("Failed to check if users are on WhatsApp: %v", err)
		return nil
	}

	for _, item := range resp {
		logMessage := fmt.Sprintf("%s: on WhatsApp: %t, JID: %s", item.Query, item.IsIn, item.JID)

		if item.VerifiedName != nil {
			logMessage += fmt.Sprintf(", business name: %s", item.VerifiedName.Details.GetVerifiedName())
		}
		log.Infof(logMessage)
		response = append(response, item)
	}
	return response
}

func handleSendTextMessage(args []string, userID int) {
	if len(args) < 2 {
		log.Errorf("Usage: send <jid> <text>")
		return
	}

	recipient, ok := parseJID(args[0])
	if !ok {
		return
	}

	msg := &waProto.Message{
		Conversation: proto.String(strings.Join(args[1:], " ")),
	}
	log.Infof("Sending message to %s: %s", recipient, msg.GetConversation())

	resp, err := cli.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		log.Errorf("Error sending message: %v", err)
		return
	}

	log.Infof("Message sent (server timestamp: %s)", resp.Timestamp)

	if err := insertMessages(resp.ID, cli.Store.ID.String(), recipient.String(), msg.GetConversation(), "text", resp.Timestamp, true, "", userID); err != nil {
		log.Errorf("Error inserting into messages: %v", err)
	}

	if err := insertLastMessages(resp.ID, cli.Store.ID.String(), recipient.String(), msg.GetConversation(), "text", resp.Timestamp, true, "", userID); err != nil {
		log.Errorf("Error inserting into last_messages: %v", err)
	}

	if wsConn != nil {
		m := Message{resp.ID, recipient.String(), "text", msg.GetConversation(), true, ""}
		wsConn.WriteJSON(m)
	}
}

func handleSendNewTextMessage(textMsg string, jid string) {
	recipient, ok := parseJID(jid)
	if !ok {
		return
	}

	msg := &waProto.Message{
		Conversation: proto.String(textMsg),
	}
	log.Infof("Sending message to %s: %s", recipient, msg.GetConversation())

	resp, err := cli.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		log.Errorf("Error sending message: %v", err)
		return
	}

	log.Infof("Message sent (server timestamp: %s)", resp.Timestamp)
}

func handleSendNewTextMessageBulk(textMsg string, jids []string) {
	var wg sync.WaitGroup
	for _, jid := range jids {
		wg.Add(1)
		go func(jid string) {
			defer wg.Done()

			recipient, ok := parseJID(jid)
			if !ok {
				return
			}

			msg := &waProto.Message{
				Conversation: proto.String(textMsg),
			}
			log.Infof("Sending message to %s: %s", recipient, msg.GetConversation())

			resp, err := cli.SendMessage(context.Background(), recipient, msg)
			if err != nil {
				log.Errorf("Error sending message: %v", err)
				return
			}

			log.Infof("Message sent (server timestamp: %s)", resp.Timestamp)
		}(jid)
	}
	wg.Wait()
}

func handleMarkRead(args []string) {
	if len(args) < 2 {
		log.Errorf("Usage: markread <message_id> <remote_jid>")
		return
	}

	messageID := args[0]
	remoteJID := args[1]

	if remoteJID == "" {
		log.Errorf("Invalid remote JID")
		return
	}

	sender, ok := parseJID(remoteJID)
	if !ok {
		return
	}

	timestamp := time.Now()

	if err := cli.MarkRead([]string{messageID}, timestamp, sender, sender); err != nil {
		log.Errorf("Error marking read: %v", err)
		return
	}
	log.Infof("MarkRead sent: %s %s %s", messageID, timestamp, sender)

	if err := markMessageRead(messageID, remoteJID, timestamp); err != nil {
		log.Errorf("Error marking message as read: %v", err)
	}
}

func handleSendImage(JID string, userID int, data []byte, captionMsg string) error {
	recipient, ok := parseJID(JID)
	if !ok {
		return fmt.Errorf("invalid JID")
	}

	uploaded, err := cli.Upload(context.Background(), data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("failed to upload file: %v", err)
	}

	msg := createImageMessage(uploaded, &data, captionMsg)
	resp, err := cli.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return fmt.Errorf("error sending image message: %v", err)
	}

	log.Infof("Image message sent (server timestamp: %s)", resp.Timestamp)

	if err := insertMessages(resp.ID, cli.Store.ID.String(), recipient.String(), "", "media", resp.Timestamp, true, "", userID); err != nil {
		return fmt.Errorf("error inserting into messages: %v", err)
	}

	if err := insertLastMessages(resp.ID, cli.Store.ID.String(), recipient.String(), "", "media", resp.Timestamp, true, "", userID); err != nil {
		return fmt.Errorf("error inserting into last_messages: %v", err)
	}

	saveImageToDisk(msg, data, resp.ID)

	if wsConn != nil {
		m := Message{resp.ID, recipient.String(), "media", "", true, ""}
		wsConn.WriteJSON(m)
	}

	return nil
}

func handleSendDocument(JID string, fileName string, userID int, data []byte, captionMsg string) error {
	recipient, ok := parseJID(JID)
	if !ok {
		return fmt.Errorf("invalid JID")
	}

	uploaded, err := cli.Upload(context.Background(), data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("failed to upload file: %v", err)
	}

	msg := createDocumentMessage(fileName, uploaded, &data, captionMsg)
	resp, err := cli.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return fmt.Errorf("error sending document message: %v", err)
	}

	log.Infof("Document message sent (server timestamp: %s)", resp.Timestamp)

	if err := insertMessages(resp.ID, cli.Store.ID.String(), recipient.String(), "", "media", resp.Timestamp, true, fileName, userID); err != nil {
		return fmt.Errorf("error inserting into messages: %v", err)
	}

	if err := insertLastMessages(resp.ID, cli.Store.ID.String(), recipient.String(), "", "media", resp.Timestamp, true, fileName, userID); err != nil {
		return fmt.Errorf("error inserting into last_messages: %v", err)
	}

	saveDocumentToDisk(msg, data, resp.ID)

	if wsConn != nil {
		m := Message{resp.ID, recipient.String(), "media", "", true, fileName}
		wsConn.WriteJSON(m)
	}

	return nil
}

func saveImageToDisk(msg *waProto.Message, data []byte, ID string) {
	exts, err := mime.ExtensionsByType(msg.GetImageMessage().GetMimetype())
	if err != nil {
		log.Errorf("Error getting file extension: %v", err)
		return
	}

	if len(exts) == 0 {
		log.Errorf("No file extension found for mimetype: %s", msg.GetImageMessage().GetMimetype())
		return
	}

	extension := exts[0]
	path := fmt.Sprintf("%s%s", ID, extension)

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		log.Errorf("Error saving file to disk: %v", err)
		return
	}

	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		log.Errorf("Error decoding image: %v", err)
		return
	}

	thumbnail := imaging.Thumbnail(img, 100, 100, imaging.Lanczos)

	thumbnailPath := fmt.Sprintf("%s%s", ID, ".jpg")
	err = imaging.Save(thumbnail, thumbnailPath, imaging.JPEGQuality(20))
	if err != nil {
		log.Errorf("Error saving thumbnail to disk: %v", err)
		return
	}

	log.Infof("Saved file to %s", path)
	log.Infof("Saved thumbnail to %s", thumbnailPath)
}

func saveDocumentToDisk(msg *waProto.Message, data []byte, ID string) {
	exts, err := mime.ExtensionsByType(msg.GetDocumentMessage().GetMimetype())
	if err != nil {
		log.Errorf("Error getting file extension: %v", err)
		return
	}

	if len(exts) == 0 {
		log.Errorf("No file extension found for mimetype: %s", msg.GetDocumentMessage().GetMimetype())
		return
	}

	extension := exts[0]
	path := fmt.Sprintf("%s%s", ID, extension)

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		log.Errorf("Error saving file to disk: %v", err)
		return
	}

	log.Infof("Saved file to %s", path)
}

func createImageMessage(uploaded whatsmeow.UploadResponse, data *[]byte, captionMsg string) *waProto.Message {
	return &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Url:           proto.String(uploaded.URL),
			Mimetype:      proto.String(http.DetectContentType(*data)),
			Caption:       &captionMsg,
			FileSha256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(*data))),
			MediaKey:      uploaded.MediaKey,
			FileEncSha256: uploaded.FileEncSHA256,
			DirectPath:    proto.String(uploaded.DirectPath),
		},
	}
}

func createDocumentMessage(fileName string, uploaded whatsmeow.UploadResponse, data *[]byte, captionMsg string) *waProto.Message {
	return &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			FileName:      proto.String(fileName),
			Url:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(http.DetectContentType(*data)),
			FileEncSha256: uploaded.FileEncSHA256,
			FileSha256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(*data))),
			Title:         proto.String(fmt.Sprintf("%s%s", "document", filepath.Ext(uploaded.URL))),
			Caption:       &captionMsg,
		},
	}
}

//save this code to remember
//func validateStringArrayAsStringArray(stringInput string) ([]string, error) {
//	//validate string is containing other than number and commas
//	stringsArr := strings.Split(stringInput, ",")
//
//	if len(stringsArr) > 1 {
//		regex := regexp.MustCompile("[^0-9,]")
//		if regex.MatchString(stringInput) {
//			return nil, errors.New("your loanIds contain other than number and commas")
//		}
//	} else if len(stringsArr) == 1 {
//		regex := regexp.MustCompile("\\d+")
//		if !regex.MatchString(stringInput) {
//			return nil, errors.New("your loanIds contain other than number and commas")
//		}
//	} else if len(stringsArr) == 0 {
//		return nil, errors.New("empty loanIds")
//	}
//
//	var stringSlice []string
//
//	for _, str := range stringsArr {
//		uintVal, err := strconv.ParseUint(strings.TrimSpace(str), 10, 64)
//		if err != nil {
//			fmt.Printf("Error parsing string %s: %v\n", str, err)
//			continue
//		}
//		uintValString := strconv.FormatUint(uintVal, 10)
//		stringSlice = append(stringSlice, uintValString)
//	}
//	return stringSlice, nil
//}

func validateStringArrayAsStringArray(stringInput string) ([]string, error) {
	// Validate if the string is empty
	if strings.TrimSpace(stringInput) == "" {
		return nil, errors.New("empty loanIds")
	}

	// Split the string by commas
	stringsArr := strings.Split(stringInput, ",")

	// Validate and clean each substring
	var stringSlice []string
	for _, str := range stringsArr {
		trimmedStr := strings.TrimSpace(str)
		// Append the converted value to the result slice
		stringSlice = append(stringSlice, trimmedStr)
	}

	return stringSlice, nil
}

func newHandleSendImage(JIDS []string, data []byte, captionMsg string) ([]Message, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var sliceM []Message
	var errs []error

	for _, jid := range JIDS {
		wg.Add(1)
		go func(jid string) {
			defer wg.Done()

			recipient, ok := parseJID(jid)
			if !ok {
				mu.Lock()
				errs = append(errs, fmt.Errorf("invalid JID: %s", jid))
				mu.Unlock()
				return
			}

			uploaded, err := cli.Upload(context.Background(), data, whatsmeow.MediaImage)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to upload file: %v", err))
				mu.Unlock()
				return
			}

			msg := createImageMessage(uploaded, &data, captionMsg)
			resp, err := cli.SendMessage(context.Background(), recipient, msg)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("error sending image message: %v", err))
				mu.Unlock()
				return
			}

			log.Infof("Image message sent (server timestamp: %s)", resp.Timestamp)

			saveImageToDisk(msg, data, resp.ID)

			m := Message{resp.ID, recipient.String(), "media", "", true, ""}
			mu.Lock()
			sliceM = append(sliceM, m)
			mu.Unlock()
		}(jid)
	}

	wg.Wait()

	// Handle errors if any
	if len(errs) > 0 {
		return nil, errs[0] // You might want to handle multiple errors differently
	}

	return sliceM, nil
}

func newHandleSendDocument(JID []string, fileName string, data []byte, captionMsg string) ([]Message, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var sliceM []Message
	var errs []error

	for _, jid := range JID {
		wg.Add(1)
		go func(jid, fileName string) {
			defer wg.Done()

			recipient, ok := parseJID(jid)
			if !ok {
				mu.Lock()
				errs = append(errs, fmt.Errorf("invalid JID: %s", jid))
				mu.Unlock()
				return
			}

			uploaded, err := cli.Upload(context.Background(), data, whatsmeow.MediaDocument)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("failed to upload file: %v", err))
				mu.Unlock()
				return
			}

			msg := createDocumentMessage(fileName, uploaded, &data, captionMsg)
			resp, err := cli.SendMessage(context.Background(), recipient, msg)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("error sending document message: %v", err))
				mu.Unlock()
				return
			}

			log.Infof("Document message sent (server timestamp: %s)", resp.Timestamp)

			saveDocumentToDisk(msg, data, resp.ID)
			m := Message{resp.ID, recipient.String(), "media", "", true, fileName}
			mu.Lock()
			sliceM = append(sliceM, m)
			mu.Unlock()
		}(jid, fileName)
	}

	wg.Wait()

	// Handle errors if any
	if len(errs) > 0 {
		return nil, errs[0] // You might want to handle multiple errors differently
	}

	return sliceM, nil
}
