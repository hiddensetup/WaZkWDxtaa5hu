package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/hiddensetup/w/app/dto"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// whatsappMessage represents the structure of a message to be sent via WhatsApp.
type whatsappMessage struct {
	Receiver string `json:"receiver"`
	Message  string `json:"message"`
	Media    string `json:"media"`
}

// SendMessage handles sending messages to a WhatsApp contact or group.
func (k *Controller) SendMessage(c *fiber.Ctx) error {
	mess := whatsappMessage{}
	if err := c.BodyParser(&mess); err != nil {
		k.client.Log.Errorf("Error parsing request body: %s", err.Error())
		return c.JSON(dto.Response{Status: false})
	}

	// Log the received JSON
	receivedJSON, err := json.Marshal(mess)
	if err != nil {
		k.client.Log.Errorf("Error marshaling JSON: %s", err.Error())
	} else {
		k.client.Log.Infof("Received JSON: %s", receivedJSON)
	}

	jid, ok := parseJID(mess.Receiver)
	if !ok {
		k.client.Log.Errorf("Invalid JID: %s", mess.Receiver)
		return c.JSON(dto.Response{Status: false})
	}

	message, err := k.makeMessage(&mess)
	if err != nil {
		k.client.Log.Errorf("Error creating WhatsApp message: %s", err.Error())
		return c.JSON(dto.Response{Status: false})
	}

	_, err = k.client.SendMessage(context.Background(), jid, message)
	if err != nil {
		// Log the error
		k.client.Log.Errorf("Error sending message: %s", err.Error())
		return c.JSON(dto.Response{Status: false})
	}

	return c.JSON(dto.Response{Status: true})
}

// LastMessage retrieves the last received WhatsApp message.
func (k *Controller) LastMessage(c *fiber.Ctx) error {
	l := len(messageList)

	if l == 0 {
		return c.SendStatus(404)
	}

	return c.JSON(messageList[l-1])
}

// makeMessage constructs a WhatsApp message based on the provided input.
func (k *Controller) makeMessage(input *whatsappMessage) (*waProto.Message, error) {
	message := waProto.Message{}

	if len(input.Media) > 0 {
		resp, err := http.Get(input.Media)
		if err != nil {
			return nil, errors.New("error getting media file by url")
		}
		defer resp.Body.Close()

		file, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.New("error reading file body")
		}

		mtype := mimetype.Detect(file)
		mimeType := mtype.String()
		mess := ""
		if len(input.Message) > 0 {
			mess = input.Message
		}
		switch mimeType {
		case "image/jpeg":
			fallthrough
		case "image/png":
			resp, err := k.client.Upload(context.Background(), file, whatsmeow.MediaImage)
			if err != nil {
				return nil, errors.New("error uploading image: " + err.Error())
			}

			message.ImageMessage = &waProto.ImageMessage{
				Caption:       proto.String(mess),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case "audio/ogg":
			mimeType = "audio/ogg; codecs=opus"
			fallthrough
		case "audio/mp3":
			fallthrough
		case "audio/mp4":
			fallthrough
		case "audio/mpeg":
			fallthrough
		case "audio/amr":
			resp, err := k.client.Upload(context.Background(), file, whatsmeow.MediaAudio)
			if err != nil {
				return nil, errors.New("error uploading file")
			}

			message.AudioMessage = &waProto.AudioMessage{
				//	Caption:       proto.String(""),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case "video/mp4":
			resp, err := k.client.Upload(context.Background(), file, whatsmeow.MediaVideo)
			if err != nil {
				return nil, errors.New("error uploading file")
			}

			message.VideoMessage = &waProto.VideoMessage{
				Caption:       proto.String(mess),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		default:
			resp, err := k.client.Upload(context.Background(), file, whatsmeow.MediaDocument)
			if err != nil {
				return nil, errors.New("error uploading file")
			}

			u, _ := url.ParseRequestURI(input.Media)

			message.DocumentMessage = &waProto.DocumentMessage{
				//Caption:       proto.String(""),
				Title:         proto.String(u.Path),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
				FileName:      proto.String(getFileName(u.Path)), // add this line to set the filename
			}
		}
	} else {
		message.Conversation = proto.String(input.Message)
	}

	return &message, nil
}

func getFileName(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func parseJID(rec string) (types.JID, bool) {
	if !strings.ContainsRune(rec, '@') {
		return types.NewJID(rec, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(rec)
		if err != nil {
			log.Printf("Invalid JID %s: %v", rec, err)
			return recipient, false
		} else if recipient.User == "" {
			log.Printf("Invalid JID %s: no server specified", rec)
			return recipient, false
		}
		log.Printf("JID OK: %s", recipient.String())
		return recipient, true
	}
}
