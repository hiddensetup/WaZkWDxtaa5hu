package controllers

import (
	"bytes"
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
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

var messageList []events.Message

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

		// Create the incoming message struct
		mess := dto.IncomingMessage{
			ID:           v.Info.ID,
			Chat:         v.Info.Chat.String(),
			Caption:      caption,
			Sender:       v.Info.Sender.String(),
			SenderName:   v.Info.PushName, // Ensure SenderName is assigned
			IsFromMe:     v.Info.IsFromMe,
			IsGroup:      v.Info.IsGroup,
			IsEphemeral:  v.IsEphemeral,
			IsViewOnce:   v.IsViewOnce,
			Timestamp:    v.Info.Timestamp.String(),
			MediaType:    v.Info.MediaType,
			Multicast:    v.Info.Multicast,
			Conversation: v.Message.GetConversation(),
		}

		if mess.Conversation == "" {
			if v.Message.ExtendedTextMessage != nil {
				mess.Conversation = v.Message.ExtendedTextMessage.GetText()
			}
		}

		// Handle attachments
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

				// Add media attachment if available for quoted message
				attachmentHandlers := map[string]func(*waE2E.Message){
					"image": func(m *waE2E.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("image", m)
					},
					"video": func(m *waE2E.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("video", m)
					},
					"audio": func(m *waE2E.Message) {
						attachment.File, _ = k.client.DownloadAny(m)
						attachment.Filename = getFilename("audio", m)
					},
					"location": func(m *waE2E.Message) {
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
						// Format for group messages with quoted content
						mess.Conversation = fmt.Sprintf("%s\n[\"%s\"]\n%s", mess.SenderName, quotedContent, mess.Conversation)
					} else {
						// If no conversation text, just include quoted content and new message
						mess.Conversation = fmt.Sprintf("%s\n[\"%s\"]\n%s", mess.SenderName, quotedContent, mess.Caption)
					}
				} else {
					// Format for individual messages with quoted content
					mess.Conversation = fmt.Sprintf("\n〚%s〛%s", quotedContent, mess.Conversation)
				}
			}
		} else if mess.MediaType != "" {
			// For non-quoted messages with media
			if mess.IsGroup {
				// If the message is from a group and has media, include media information and new message
				mess.Conversation = fmt.Sprintf("%s\n%s", mess.Conversation, mess.Caption)
			}
		}

		if v.Message.ContactMessage != nil {
			s := *v.Message.ContactMessage.Vcard

			// Helper function to extract values based on the pattern
			extractValue := func(pattern string, s string) string {
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(s)
				if len(matches) > 1 {
					return strings.TrimSpace(matches[1])
				}
				return ""
			}

			waPhone := extractValue(`TEL.*?:(.+)`, s)
			waPhone = strings.ReplaceAll(waPhone, " ", "") // remove spaces from phone number
			waPhone = strings.ReplaceAll(waPhone, "-", "") // remove dashes from phone number

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

			// Handle replies to location messages
			if v.Message.ExtendedTextMessage != nil && v.Message.ExtendedTextMessage.ContextInfo != nil {
				if v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage != nil {
					quotedMsg := v.Message.ExtendedTextMessage.ContextInfo.QuotedMessage
					if quotedMsg.LocationMessage != nil {
						// If the quoted message is a location message, include the location URL in the reply
						mess.Conversation = fmt.Sprintf("%s\nReplying to location: %s", mess.Conversation, locationUrl)
					}
				} else {
					// If replying to a non-quoted location message
					mess.Conversation = fmt.Sprintf("%s\n%s", mess.Conversation, locationUrl)
				}
			} else {
				// If the message is a location message itself, include the URL
				mess.Conversation = "\n" + locationUrl
			}
			mess.Caption = locationUrl
		}

		// Adjust message formatting for group messages
		if mess.IsGroup {
			if v.Message.ExtendedTextMessage != nil {
				if v.Message.ExtendedTextMessage.ContextInfo == nil {
					// Handle group messages without quoted content
					if mess.IsFromMe {
						// If the message is from the current user (sender), format as "SenderName (Me)"
						mess.Conversation = fmt.Sprintf("%s (Me) \n%s", mess.SenderName, mess.Conversation)
					} else {
						// For messages from others, format as "SenderName"
						mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, mess.Conversation)
					}
				}
			} else if mess.MediaType != "" {
				// If no ExtendedTextMessage and has media
				mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, mess.Conversation)
			} else {
				// If no ExtendedTextMessage and no media
				mess.Conversation = fmt.Sprintf("%s\n%s", mess.SenderName, mess.Conversation)
			}
		}

		if mess.Chat != "status@broadcast" {
			k.proxyToChatApp(mess, attachment)
		}
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

func getFilename(mediaType string, message *waE2E.Message) string {
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
