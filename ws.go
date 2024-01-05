package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/mdp/qrterminal/v3"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Command struct {
	Cmd       string   `json:"cmd"`
	Arguments []string `json:"args"`
	UserID    int      `json:"user_id"`
}

// Handle incoming WebSocket connections, read json messages and pass them to the handleCmd function
func serveWs(w http.ResponseWriter, r *http.Request) {
	var err error
	wsConn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade connection: %v", err)
		return
	}
	defer wsConn.Close()

	for {
		var cmd Command
		err := wsConn.ReadJSON(&cmd)
		if err != nil {
			log.Errorf("Failed to read json: %v", err)
			return
		}
		handleCmd(cmd)
	}
}

// ServeStatus returns the current status of the client
func serveSendText(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if cli.IsLoggedIn() {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		type messageBodyText struct {
			Recipient string `json:"recipient" validate:"required"`
			Message   string `json:"message" validate:"required"`
		}

		var msgBody messageBodyText
		err = json.Unmarshal(body, &msgBody)
		if err != nil {
			http.Error(w, "Error decoding JSON", http.StatusBadRequest)
			return
		}
		handleSendNewTextMessage(msgBody.Message, msgBody.Recipient)

		respJson, err := json.Marshal(msgBody)
		if err != nil {
			http.Error(w, "Error decoding JSON", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respJson)
		return

	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// ServeStatus returns the current status of the client
func serveSendTextBulk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if cli.IsLoggedIn() {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		type messageBodyText struct {
			Recipient []string `json:"recipient" validate:"required"`
			Message   string   `json:"message" validate:"required"`
		}

		var msgBody messageBodyText
		err = json.Unmarshal(body, &msgBody)
		if err != nil {
			http.Error(w, "Error decoding JSON", http.StatusBadRequest)
			return
		}
		handleSendNewTextMessageBulk(msgBody.Message, msgBody.Recipient)

		respJson, err := json.Marshal(msgBody)
		if err != nil {
			http.Error(w, "Error decoding JSON", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respJson)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

func serveCheckUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if cli.IsLoggedIn() {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusBadRequest)
			return
		}

		type messageBodyText struct {
			Recipient []string `json:"recipient" validate:"required"`
		}

		var msgBody messageBodyText
		err = json.Unmarshal(body, &msgBody)
		if err != nil {
			http.Error(w, "Error decoding JSON", http.StatusBadRequest)
			return
		}
		response := newHandleCheckUser(msgBody.Recipient)

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// ServeStatus returns the current status of the client
func serveStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if cli.IsLoggedIn() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := struct {
			ID       string `json:"id"`
			PushName string `json:"pushName"`
			IsLogin  bool   `json:"isLogin"`
		}{
			ID:       cli.Store.ID.String(),
			PushName: cli.Store.PushName,
			IsLogin:  cli.IsLoggedIn(),
		}

		// Convert the response to JSON
		jsonResponse, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}

		w.Write(jsonResponse)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

func serveQR(w http.ResponseWriter, r *http.Request) {
	if cli.IsLoggedIn() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	qrterminal.GenerateHalfBlock(qrStr, qrterminal.L, w)
}

func uploadHandler(w http.ResponseWriter, r *http.Request, uploadDir string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		handleError(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		handleError(w, http.StatusBadRequest, "Failed to retrieve file from request", err)
		return
	}
	defer file.Close()

	JID := r.FormValue("jid")
	userID, err := strconv.Atoi(r.FormValue("user_id"))
	if err != nil {
		handleError(w, http.StatusBadRequest, "Invalid user ID", err)
		return
	}

	captionMsg := r.FormValue("caption")

	data, err := io.ReadAll(file)
	if err != nil {
		handleError(w, http.StatusInternalServerError, "Failed to read file data", err)
		return
	}

	mimeType := http.DetectContentType(data)

	extAsImage := []string{"image/jpeg", "image/png"}
	if stringContains(extAsImage, mimeType) {
		err = handleSendImage(JID, userID, data, captionMsg)
		if err != nil {
			handleError(w, http.StatusInternalServerError, "Failed to handle image upload", err)
			return
		}
	} else {
		err = handleSendDocument(JID, handler.Filename, userID, data, captionMsg)
		if err != nil {
			handleError(w, http.StatusInternalServerError, "Failed to handle document upload", err)
			return
		}
	}

	log.Infof("Uploaded file %s to %s, mimetype: %s", handler.Filename, JID, mimeType)
	w.WriteHeader(http.StatusOK)
}

func newUploadHandler(w http.ResponseWriter, r *http.Request, uploadDir string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		handleError(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}

	// Get the files
	files, ok := r.MultipartForm.File["file"]
	if !ok || len(files) == 0 {
		handleError(w, http.StatusBadRequest, "No files found in the request", nil)
		return
	}

	JID := r.FormValue("jid")
	captionMsg := r.FormValue("caption")

	var resp []Message

	for _, handler := range files {
		// Open the file
		file, err := handler.Open()
		if err != nil {
			handleError(w, http.StatusInternalServerError, "Failed to open file", err)
			return
		}
		defer file.Close()

		// Read the file data
		data, err := io.ReadAll(file)
		if err != nil {
			handleError(w, http.StatusInternalServerError, "Failed to read file data", err)
			return
		}

		sliceJID, err := validateStringArrayAsStringArray(JID)
		if err != nil {
			handleError(w, http.StatusInternalServerError, "Something went wrong with parameter jid", err)
			return
		}

		var uploadResp []Message
		mimeType := http.DetectContentType(data)
		if isImage(mimeType) {
			uploadResp, err = newHandleSendImage(sliceJID, data, captionMsg)
		} else {
			uploadResp, err = newHandleSendDocument(sliceJID, handler.Filename, data, captionMsg)
		}

		if err != nil {
			handleError(w, http.StatusInternalServerError, "Failed to handle file upload", err)
			return
		}

		resp = append(resp, uploadResp...)
	}

	jsonResponse, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)
}

func handleError(w http.ResponseWriter, statusCode int, message string, err error) {
	log.Errorf("%s: %v", message, err)
	http.Error(w, message, statusCode)
}

func isImage(mimeType string) bool {
	extAsImage := []string{"image/jpeg", "image/png"}
	return stringContains(extAsImage, mimeType)
}

func stringContains(strSlice []string, str string) bool {
	for _, val := range strSlice {
		if val == str {
			return true
		}
	}
	return false
}
