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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hiddensetup/w/app/dto"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types/events"
)

var messageList []events.Message
var enableGroupHandling bool = true

func (k *Controller) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		messageList = append(messageList, *v)

		caption := ""
		if v.Message.ImageMessage != nil {
			if v.Message.ImageMessage.Caption != nil {
				caption = *v.Message.ImageMessage.Caption
			}
		}
		if v.Message.VideoMessage != nil {
			if v.Message.VideoMessage.Caption != nil {
				caption = *v.Message.VideoMessage.Caption
			}
		}

		mess := dto.IncomingMessage{
			ID:           v.Info.ID,
			Chat:         v.Info.Chat.String(),
			Caption:      caption,
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

		// Check if the message is forwarded
		isForwarded := false
		if v.Message.ExtendedTextMessage != nil &&
			v.Message.ExtendedTextMessage.ContextInfo != nil &&
			v.Message.ExtendedTextMessage.ContextInfo.IsForwarded != nil &&
			*v.Message.ExtendedTextMessage.ContextInfo.IsForwarded {
			isForwarded = true
		}

		if mess.Conversation == "" {
			if v.Message.ExtendedTextMessage != nil {
				mess.Conversation = v.Message.ExtendedTextMessage.GetText()
			}
		}

		var attachment dto.MessageAttachment
		if mess.MediaType != "" {
			attachment.File, _ = k.client.DownloadAny(v.Message)
			attachment.Filename = getFilename(v.Info.MediaType, v.Message)
		}

		// Handle quoted messages
		if v.Message.ExtendedTextMessage != nil && v.Message.ExtendedTextMessage.ContextInfo != nil {
			if v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
				quotedMsg := v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage
				quotedConversation := ""
				if quotedMsg.Conversation != nil {
					quotedConversation = *quotedMsg.Conversation
				}
				participant := ""
				if v.Message.ExtendedTextMessage.ContextInfo.Participant != nil {
					participant = *v.Message.ExtendedTextMessage.ContextInfo.Participant
				}

				quotedContent := fmt.Sprintf("%s\n%s", participant, quotedConversation)

				attachmentHandlers := map[string]func(*waProto.Message){
					"image": func(m *waProto.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("image", m)
					},
					"video": func(m *waProto.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("video", m)
					},
					"audio": func(m *waProto.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("audio", m)
					},
					"location": func(m *waProto.Message) {
						mapsUrl := "https://maps.google.com"
						latitude := *m.LocationMessage.DegreesLatitude
						longitude := *m.LocationMessage.DegreesLongitude
						locationUrl := fmt.Sprintf("%s/?q=%f,%f", mapsUrl, latitude, longitude)
						quotedContent = fmt.Sprintf("%s %s\n", quotedContent, locationUrl)
					},
				}

				if quotedMsg.ImageMessage != nil {
					attachmentHandlers["image"](quotedMsg)
				}
				if quotedMsg.VideoMessage != nil {
					attachmentHandlers["video"](quotedMsg)
				}
				if quotedMsg.AudioMessage != nil {
					attachmentHandlers["audio"](quotedMsg)
				}
				if quotedMsg.LocationMessage != nil {
					attachmentHandlers["location"](quotedMsg)
				}

				if mess.IsGroup {
					if mess.Conversation != "" {
						mess.Conversation = fmt.Sprintf("%s\n[\"%s\"]\n%s", mess.SenderName, quotedContent, mess.Conversation)
					} else {
						mess.Conversation = fmt.Sprintf("%s\n[\"%s\"]\n%s", mess.SenderName, quotedContent, mess.Caption)
					}
				} else {
					mess.Conversation = fmt.Sprintf("\n〚%s〛%s", quotedContent, mess.Conversation)
				}
			}
		}

		// Handle forwarded messages
		if isForwarded {
			forwardedPrefix := "→Forwarded←\n"
			if mess.IsGroup {
				if mess.Conversation != "" {
					mess.Conversation = fmt.Sprintf("%s%s\n%s", forwardedPrefix, mess.SenderName, mess.Conversation)
				} else if mess.Caption != "" {
					mess.Caption = fmt.Sprintf("%s%s\n%s", forwardedPrefix, mess.SenderName, mess.Caption)
				}
			} else {
				mess.Conversation = forwardedPrefix + mess.Conversation
			}
		}

		if v.Message.ReactionMessage != nil && v.Message.ReactionMessage.Text != nil {
			reaction := *v.Message.ReactionMessage.Text
			mess.Conversation = fmt.Sprintf("%s", reaction)
		}

		if v.Message.ContactMessage != nil {
			s := *v.Message.ContactMessage.Vcard

			extractValue := func(pattern string, s string) string {
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(s)
				if len(matches) > 1 {
					return strings.TrimSpace(matches[1])
				}
				return ""
			}

			waPhone := extractValue(`TEL.*?:(.+)`, s)
			waPhone = strings.ReplaceAll(waPhone, " ", "")
			waPhone = strings.ReplaceAll(waPhone, "-", "")

			waEmail := extractValue(`EMAIL.*?:(.+)`, s)
			contactName := *v.Message.ContactMessage.DisplayName

			mess.Conversation = fmt.Sprintf("*%s*\n%s\n%s", contactName, waPhone, waEmail)
			mess.Caption = contactName
		}

		if v.Message.LocationMessage != nil {
			mapsUrl := "https://maps.google.com"
			latitude := *v.Message.LocationMessage.DegreesLatitude
			longitude := *v.Message.LocationMessage.DegreesLongitude

			locationUrl := fmt.Sprintf("%s/?q=%f,%f", mapsUrl, latitude, longitude)

			if v.Message.ExtendedTextMessage != nil && v.Message.ExtendedTextMessage.ContextInfo != nil {
				if v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
					quotedMsg := v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage
					if quotedMsg.LocationMessage != nil {
						mess.Conversation = fmt.Sprintf("%s\nReply: %s", mess.Conversation, locationUrl)
					}
				} else {
					mess.Conversation = fmt.Sprintf("%s\n%s", mess.Conversation, locationUrl)
				}
			} else {
				mess.Conversation = "\n" + locationUrl
			}
			mess.Caption = locationUrl
		}

		if mess.IsGroup && enableGroupHandling {
			if v.Message.ExtendedTextMessage != nil {
				if v.Message.ExtendedTextMessage.ContextInfo == nil {
					if mess.IsFromMe {
						mess.Conversation = fmt.Sprintf("%s (Me) \n%s", mess.SenderName, mess.Conversation)
					} else {
						mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, mess.Conversation)
					}
				}
			} else if mess.MediaType != "" {
				if caption != "" {
					mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, caption)
				} else {
					mess.Conversation = mess.SenderName
				}
			} else {
				mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, mess.Conversation)
			}
		}

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
		"sticker": func() string { return hash(message.StickerMessage.String()) + ".webp" },
		"gif":     func() string { return hash(message.VideoMessage.String()) + ".mp4" },
		"image": func() string {
			mimeType := message.ImageMessage.GetMimetype()
			ext, exists := mimeToExt[mimeType]
			if !exists {
				ext = "png" // default to jpg if MIME type is unknown
			}
			return hash(message.ImageMessage.String()) + "." + ext
		},
		"video": func() string { return hash(message.VideoMessage.String()) + ".mp4" },
		"document": func() string {
			if message.DocumentMessage != nil {
				return message.DocumentMessage.GetFileName()
			}
			return ""
		},
		"vcard": func() string { return message.ContactMessage.GetDisplayName() + ".vcf" },
		"ptt":   func() string { return hash(message.AudioMessage.String()) + ".ogg" },
		"audio": func() string { return hash(message.AudioMessage.String()) + ".mp3" },
		"product": func() string {
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
