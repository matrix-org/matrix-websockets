// Package proxy request.go contains functions for dealing with requests receieved from
// websocket clients.
package proxy

import (
	"encoding/json"
	"log"
)

type jsonRequest struct {
	ID     *string
	Method string
	Params *json.RawMessage
}

type jsonResponse struct {
	// this is a pointer so that it can be set to 'nil' to give a null result
	ID *string `json:"id"`

	Result resultObj `json:"result,omitempty"`

	// this is a pointer so that we can set it to be nil to omit it from the output.
	Error *MatrixErrorDetails `json:"error,omitempty"`
}

type resultObj interface{}

type requestHandler func(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails)

var handlerMap = map[string]requestHandler{
	"ping":         handlePing,
	"read_markers": handleReadMarkers,
	"send":         handleSend,
	"state":        handleState,
	"typing":       handleTyping,
}

// handleRequest gets the correct response for a received message, and returns
// the json encoding
func handleRequest(request []byte, client *MatrixClient) []byte {
	var resp *jsonResponse
	var jr jsonRequest

	if err := json.Unmarshal(request, &jr); err != nil {
		log.Println("Invalid request:", err)
		resp = &jsonResponse{
			Error: &MatrixErrorDetails{
				ErrCode: "M_NOT_JSON",
				Error:   err.Error(),
			},
		}
	} else {
		result, errdetails := handleRequestObject(&jr, client)
		resp = &jsonResponse{
			ID:     jr.ID,
			Result: result,
			Error:  errdetails,
		}
	}

	v, err := json.Marshal(resp)
	if err != nil {
		log.Print("Error marshalling:", err)
		return nil
	}
	return v
}

func handleRequestObject(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails) {
	// treat absent params the same as empty ones
	if req.Params == nil {
		req.Params = new(json.RawMessage)
		req.Params.UnmarshalJSON([]byte("{}"))
	}

	handler, ok := handlerMap[req.Method]

	if ok {
		return handler(req, client)
	}

	// unknown method
	log.Println("Unknown method:", req.Method)
	return nil, &MatrixErrorDetails{
		ErrCode: "M_BAD_JSON",
		Error:   "Unknown method",
	}
}

func handlePing(req *jsonRequest, _ *MatrixClient) (resultObj, *MatrixErrorDetails) {
	return &struct{}{}, nil
}

func handleReadMarkers(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails) {
	type ReadMarkerRequest struct {
		RoomID    string `json:"room_id"`
		FullyRead string `json:"m.fully_read,omitempty"`
		Read      string `json:"m.read,omitempty"`
	}
	type ReadMarkerUpstreamRequest struct {
		FullyRead string `json:"m.fully_read,omitempty"`
		Read      string `json:"m.read,omitempty"`
	}

	var readMarkerParams ReadMarkerRequest
	if err := json.Unmarshal(*req.Params, &readMarkerParams); err != nil {
		log.Println("Invalid request:", err)
		return nil, errorToResponse(err)
	}

	if readMarkerParams.RoomID == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing room_id",
		}
	}

	if readMarkerParams.FullyRead == "" && readMarkerParams.Read == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "either m.read or m.fully_read are required",
		}
	}

	jsonContent, err := json.Marshal(ReadMarkerUpstreamRequest{
		FullyRead: readMarkerParams.FullyRead,
		Read:      readMarkerParams.Read,
	})

	if err != nil {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Intermediate content not parseable",
		}
	}
	_, err = client.SendReadMarkers(readMarkerParams.RoomID, jsonContent)

	if err != nil {
		return nil, errorToResponse(err)
	}

	return &struct{}{}, nil
}

func handleSend(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails) {
	type SendRequest struct {
		RoomID    string `json:"room_id"`
		EventType string `json:"event_type"`
		Content   *json.RawMessage
	}
	type SendResponse struct {
		EventID string `json:"event_id"`
	}

	if req.ID == nil {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing request ID",
		}
	}

	var sendParams SendRequest
	if err := json.Unmarshal(*req.Params, &sendParams); err != nil {
		log.Println("Invalid request:", err)
		return nil, errorToResponse(err)
	}

	if sendParams.RoomID == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing room_id",
		}
	}

	if sendParams.EventType == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing event_type",
		}
	}

	if sendParams.Content == nil {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing content",
		}
	}

	eventID, err := client.SendMessage(sendParams.RoomID, sendParams.EventType, *req.ID,
		*sendParams.Content)

	if err != nil {
		return nil, errorToResponse(err)
	}

	return &SendResponse{EventID: eventID}, nil
}

func handleState(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails) {
	type StateRequest struct {
		RoomID    string `json:"room_id"`
		EventType string `json:"event_type"`
		StateKey  string `json:"state_key"`
		Content   *json.RawMessage
	}
	type StateResponse struct {
		EventID string `json:"event_id"`
	}

	var stateParams StateRequest
	if err := json.Unmarshal(*req.Params, &stateParams); err != nil {
		log.Println("Invalid request:", err)
		return nil, errorToResponse(err)
	}

	if stateParams.RoomID == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing room_id",
		}
	}

	if stateParams.EventType == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing event_type",
		}
	}

	if stateParams.Content == nil {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing content",
		}
	}

	eventID, err := client.SendState(stateParams.RoomID, stateParams.EventType,
		stateParams.StateKey, *stateParams.Content)

	if err != nil {
		return nil, errorToResponse(err)
	}

	return &StateResponse{EventID: eventID}, nil
}

func handleTyping(req *jsonRequest, client *MatrixClient) (resultObj, *MatrixErrorDetails) {
	type TypingRequest struct {
		RoomID  string `json:"room_id"`
		Typing  bool   `json:"typing"`
		Timeout int    `json:"timeout,omitempty"`
	}

	var typingParams TypingRequest
	if err := json.Unmarshal(*req.Params, &typingParams); err != nil {
		log.Println("Invalid request:", err)
		return nil, errorToResponse(err)
	}

	if typingParams.RoomID == "" {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Missing room_id",
		}
	}

	content := TypingRequest{
		RoomID: typingParams.RoomID,
		Typing: typingParams.Typing,
	}
	if typingParams.Typing && typingParams.Timeout != 0 {
		content.Timeout = typingParams.Timeout
	}

	jsonContent, err := json.Marshal(&content)
	if err != nil {
		return nil, &MatrixErrorDetails{
			ErrCode: "M_BAD_JSON",
			Error:   "Intermediate content not parseable",
		}

	}
	_, err = client.SendTyping(typingParams.RoomID, jsonContent)

	if err != nil {
		return nil, errorToResponse(err)
	}

	return &struct{}{}, nil
}

// errorToResponse takes an error object and turns it into our best MatrixErrorDetails
func errorToResponse(err error) *MatrixErrorDetails {
	switch err.(type) {
	case *MatrixError:
		return &err.(*MatrixError).Details
	default:
		return &MatrixErrorDetails{
			ErrCode: "M_UNKNOWN",
			Error:   err.Error(),
		}
	}
}
