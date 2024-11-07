package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/hiddensetup/WaZkWDxtaa5hu/app/dto"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
)

var messageList []events.Message
var enableGroupHandling bool = true

func (k *Controller) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		messageList = append(messageList, *v)

		// Create a DTO to represent the incoming message
		mess := dto.IncomingMessage{
			ID:           v.Info.ID,
			Chat:         v.Info.Chat.String(),
			Caption:      "",
			Sender:       v.Info.Sender.String(),
			SenderName:   v.Info.PushName,
			IsFromMe:     v.Info.IsFromMe,
			IsGroup:      v.Info.IsGroup && enableGroupHandling,
			IsEphemeral:  v.IsEphemeral,
			IsViewOnce:   v.IsViewOnce,
			Timestamp:    v.Info.Timestamp.String(),
			MediaType:    v.Info.MediaType,
			Multicast:    v.Info.Multicast,
			Conversation: v.Message.GetConversation(),
		}

		// Extract conversation text from extended text messages
		if mess.Conversation == "" {
			if v.Message.ExtendedTextMessage != nil {
				mess.Conversation = v.Message.ExtendedTextMessage.GetText()
			}
		}

		// Handle message attachments
		var attachment dto.MessageAttachment
		if mess.MediaType != "" {
			attachment.File, _ = k.client.DownloadAny(v.Message)
			attachment.Filename = getFilename(v.Info.MediaType, v.Message)
		}

		// Proxy message to chat app if not a status broadcast
		if mess.Chat != "status@broadcast" {
			k.proxyToChatApp(mess, attachment)
		}

		// Print JSON representation of the message
		messageJSON, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			k.client.Log.Errorf("Error marshalling message to JSON: %s", err)
		} else {
			fmt.Printf("Message JSON:\n%s\n", string(messageJSON))
		}

		// Print formatted message content
		fmt.Printf("Formatted Message:\n%s\n", mess.Conversation)
	}
}

func (k *Controller) proxyToChatApp(message dto.IncomingMessage, attachment ...dto.MessageAttachment) string {
	client := &http.Client{Timeout: time.Second * 10}

	// New multipart writer.
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Encode message fields.
	if err := encodeFields(writer, message); err != nil {
		k.client.Log.Errorf("Encoding message fields error: %s", err)
		return ""
	}

	// Handle attachment if provided.
	if len(attachment) > 0 && !attachment[0].IsEmpty() {
		if err := addAttachment(writer, attachment[0]); err != nil {
			k.client.Log.Errorf("Adding attachment error: %s", err)
			return ""
		}
	}

	writer.Close()

	// Create and send request.
	req, err := http.NewRequest("POST", os.Getenv("PROXY_URL"), body)
	if err != nil {
		k.client.Log.Errorf("Creating request error: %s", err)
		return ""
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		k.client.Log.Errorf("Request error or status not OK: %s, status: %d", err, resp.StatusCode)
		return ""
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		k.client.Log.Errorf("Reading response body error: %s", err)
		return ""
	}

	return string(content)
}

func encodeFields(writer *multipart.Writer, message dto.IncomingMessage) error {
	v := reflect.ValueOf(message)
	typeOfS := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fw, err := writer.CreateFormField(typeOfS.Field(i).Name)
		if err != nil {
			return err
		}
		if _, err = io.Copy(fw, strings.NewReader(fmt.Sprintf("%v", v.Field(i).Interface()))); err != nil {
			return err
		}
	}
	return nil
}

func addAttachment(writer *multipart.Writer, attachment dto.MessageAttachment) error {
	fw, err := writer.CreateFormFile("attachment", attachment.Filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, bytes.NewReader(attachment.File))
	return err
}

func getFilename(mediaType string, message *waProto.Message) string {
	type fileExtractor func() string

	// Map MIME types to file extensions
	mimeToExt := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/gif":  "gif",
		"video/mp4":  "mp4",
		"audio/ogg":  "ogg",
		"audio/mp3":  "mp3",
	}

	// File extractors for different media types
	extractors := map[string]fileExtractor{
		"sticker": func() string {
			// Handle sticker attachment
			return hash(message.StickerMessage.String()) + ".webp"
		},
		"gif": func() string {
			// Handle GIF attachment
			return hash(message.VideoMessage.String()) + ".mp4"
		},
		"image": func() string {
			// Handle image attachment
			mimeType := message.ImageMessage.GetMimetype()
			ext, exists := mimeToExt[mimeType]
			if !exists {
				ext = "png" // default to jpg if MIME type is unknown
			}
			return hash(message.ImageMessage.String()) + "." + ext
		},
		"video": func() string {
			// Handle video attachment
			return hash(message.VideoMessage.String()) + ".mp4"
		},
		"document": func() string {
			// Handle document attachment
			if message.DocumentMessage != nil {
				return message.DocumentMessage.GetFileName()
			}
			return ""
		},
		"vcard": func() string {
			// Handle vCard attachment
			return message.ContactMessage.GetDisplayName() + ".vcf"
		},
		"ptt": func() string {
			// Handle PTT (voice note) attachment
			return hash(message.AudioMessage.String()) + ".ogg"
		},
		"audio": func() string {
			// Handle audio attachment
			return hash(message.AudioMessage.String()) + ".mp3"
		},
		"product": func() string {
			// Handle product attachment
			if message.ProductMessage != nil {
				return message.ProductMessage.String() + ".jpg"
			}
			return ""
		},
	}

	if extractor, exists := extractors[mediaType]; exists {
		return extractor()
	}

	return ""
}

func hash(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	return strconv.FormatUint(uint64(h.Sum32()), 10)
}
