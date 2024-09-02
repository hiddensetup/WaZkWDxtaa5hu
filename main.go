package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/hiddensetup/w/app/controllers"
	"github.com/hiddensetup/w/app/routes"
	"github.com/joho/godotenv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Error loading .env file")
	}

	binaryName := os.Getenv("BINARY_NAME")
	if binaryName == "" {
		log.Fatal("BINARY_NAME not set in .env file")
	}

	// Check and rename if necessary
	if err := checkAndRenameBinary(binaryName); err != nil {
		log.Fatal("Error checking or renaming binary:", err)
	}

	// Write the PID to the .env file
	if err := updatePIDInEnv(); err != nil {
		log.Fatal("Error updating PID in .env file:", err)
	}

	app := fiber.New()

	dbLog := waLog.Stdout("Database", os.Getenv("LOG_LEVEL"), true)

	dbContainer, err := sqlstore.New("sqlite3", "file:whatsappstore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	controller := controllers.NewController(dbContainer)
	defer controller.GetClient().Disconnect()

	routes.Setup(app, controller)

	if os.Getenv("AUTO_LOGIN") == `1` {
		if err := controller.Autologin(); err != nil {
			log.Fatal("Error auto connect WhatsApp")
		}
	}

	// Use ListenTLS for HTTPS
	certFile := os.Getenv("SSL_CERT_FILE")
	keyFile := os.Getenv("SSL_KEY_FILE")

	if certFile == "" || keyFile == "" {
		log.Fatal("SSL_CERT_FILE and SSL_KEY_FILE must be set in .env file")
	}

	if err := app.ListenTLS(fmt.Sprintf(":%s", os.Getenv("PORT")), certFile, keyFile); err != nil {
		fmt.Println("new error emitted: ", err)
		log.Fatal("error starting https server")
	}
}

func updatePIDInEnv() error {
	pid := os.Getpid()
	pidStr := fmt.Sprintf("PID=%d\n", pid)

	// Read the existing .env file and write back to it, excluding old PID lines
	file, err := os.Open(".env")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var lines []string
	if err == nil {
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "PID=") {
				lines = append(lines, line)
			}
		}
		file.Close()
		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// Open the .env file for writing
	file, err = os.OpenFile(".env", os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the lines back to the file
	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	// Write the new PID to the file
	if _, err := file.WriteString(pidStr); err != nil {
		return err
	}

	return nil
}

func checkAndRenameBinary(expectedName string) error {
	currentPath, err := os.Executable()
	if err != nil {
		return err
	}

	currentName := filepath.Base(currentPath)

	if currentName != expectedName {
		newPath := filepath.Join(filepath.Dir(currentPath), expectedName)
		if err := os.Rename(currentPath, newPath); err != nil {
			return err
		}

		// Restart the application with the new name
		cmd := fmt.Sprintf("%s %s", newPath, strings.Join(os.Args[1:], " "))
		if err := executeCommand(cmd); err != nil {
			return err
		}
		os.Exit(0) // Exit the current process
	}

	return nil
}

func executeCommand(cmd string) error {
	// Use exec.Command to execute the new command
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return fmt.Errorf("no command to execute")
	}

	command := parts[0]
	args := parts[1:]

	c := exec.Command(command, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin

	return c.Run()
}
