package controllers

import (
	"context"
	"log"
	"os"
	"os/exec"

	"github.com/gofiber/fiber/v2"
	"github.com/hiddensetup/w/app/dto"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type Controller struct {
	dbContainer *sqlstore.Container
	client      *whatsmeow.Client
	qrCode      string // Updated to instance variable
}

func NewController(db *sqlstore.Container) *Controller {
	cntrl := &Controller{
		dbContainer: db,
		qrCode:      "", // Initialize qrCode
	}

	clientLog := waLog.Stdout("Client", os.Getenv("LOG_LEVEL"), true)
	cntrl.client = whatsmeow.NewClient(cntrl.getDevice(), clientLog)
	cntrl.client.AddEventHandler(cntrl.eventHandler)

	return cntrl
}

func (k *Controller) Login(c *fiber.Ctx) error {
	if k.client.Store.ID == nil {
		// No ID stored, new login
		if !k.client.IsConnected() {
			if err := k.client.Connect(); err != nil {
				k.client.Log.Errorf("WhatsApp connection error: %s", err.Error())
				return c.SendStatus(500)
			}
		}

		k.client.Disconnect() // Disconnect before reconnecting

		qrChan, err := k.client.GetQRChannel(context.Background())
		if err != nil {
			k.client.Log.Errorf("WhatsApp connection error: %s", err.Error())
			return c.SendStatus(500)
		}

		if err := k.client.Connect(); err != nil {
			k.client.Log.Errorf("WhatsApp connection error: %s", err.Error())
			return c.SendStatus(500)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				k.qrCode = evt.Code

				if k.qrCode != "" {
					qrCodeImg, err := qrcode.Encode(k.qrCode, qrcode.Medium, 500)
					if err != nil {
						k.client.Log.Errorf("QR code generation error: %s", err.Error())
						return c.SendStatus(500)
					}

					return c.Send(qrCodeImg)
				}
			} else {
				break // Exit loop on any event other than "code"
			}
		}
	} else {
		// Already logged in, just connect
		if err := k.Autologin(); err != nil {
			k.client.Log.Errorf("WhatsApp connection error: %s", err.Error())
			return c.SendStatus(500)
		}

		return c.JSON(dto.Response{Status: true})
	}

	return c.JSON(dto.Response{Status: false})
}

func (k *Controller) Autologin() error {
	// autologin only when client is auth
	if k.client.Store.ID != nil && !k.client.IsConnected() {
		err := k.client.Connect()
		if err != nil {
			k.client.Log.Errorf("WhatsApp connection error: %s", err.Error())
			return err
		}
	}

	return nil
}
func (k *Controller) Logout(c *fiber.Ctx) error {
	// Remove the whatsappstore.db file if it exists
	if _, err := os.Stat("whatsappstore.db"); err == nil {
		if err := os.Remove("whatsappstore.db"); err != nil {
			// Handle the error more gracefully, log it, and continue
			log.Printf("Error removing whatsappstore.db: %s", err)
		}
	}

	// Log out the user
	if k.client != nil {
		if err := k.client.Logout(); err != nil {
			// Handle the error more gracefully, return an error response
			return c.JSON(dto.Response{Status: false})
		}
	}

	return c.JSON(dto.Response{Status: true})
}

func (k *Controller) ExecuteScript(c *fiber.Ctx) error {
	scriptPath := "./start.sh"

	// Execute the script
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return c.JSON(dto.Response{Status: false})
	}

	return c.JSON(dto.Response{Status: true})
}

func (k *Controller) Off(c *fiber.Ctx) error {
	scriptPath := "./off.sh"

	// Execute the script
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return c.JSON(dto.Response{Status: false})
	}

	return c.JSON(dto.Response{Status: true})
}

func (k *Controller) getDevice() *store.Device {
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := k.dbContainer.GetFirstDevice()

	if err != nil {
		k.client.Log.Errorf("Device getting error: %s", err.Error())

		return nil
	}

	return deviceStore
}

func (k *Controller) GetClient() *whatsmeow.Client {
	return k.client
}
