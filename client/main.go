// client.go
package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/ProtonMail/gopenpgp/v3/profile"
	"github.com/gdamore/tcell/v2"
	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

type WSMessage struct {
	Type      string `json:"type"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`
	Payload   string `json:"payload,omitempty"`
}

type Client struct {
	conn       *websocket.Conn
	username   string
	privateKey *crypto.Key
	publicKey  *crypto.Key
	publicKeys map[string]*crypto.Key // Map of usernames to their public keys
	app        *tview.Application
	flex       *tview.Flex
	inputField *tview.InputField
	textView   *tview.TextView
	usersView  *tview.TextView
}

func NewClient() *Client {
	return &Client{
		publicKeys: make(map[string]*crypto.Key),
	}
}

func (c *Client) generateKeyPair(username string) error {
	pgp := crypto.PGPWithProfile(profile.RFC4880())

	keyGenHandle := pgp.KeyGeneration().AddUserId(username, username+"@chat.local").New()
	privateKey, err := keyGenHandle.GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %v", err)
	}

	publicKey, err := privateKey.ToPublic()
	if err != nil {
		return fmt.Errorf("failed to get public key: %v", err)
	}

	c.privateKey = privateKey
	c.publicKey = publicKey

	return nil
}

func (c *Client) connectToServer(serverURL, username string) error {
	// Generate key pair
	if err := c.generateKeyPair(username); err != nil {
		return err
	}

	// Connect to WebSocket server
	u, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("failed to parse server URL: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}

	c.conn = conn
	c.username = username

	// Get armored public key
	publicKeyArmored, err := c.publicKey.Armor()
	if err != nil {
		return fmt.Errorf("failed to armor public key: %v", err)
	}

	// Send join message
	joinMsg := WSMessage{
		Type:      "join",
		From:      username,
		PublicKey: publicKeyArmored,
	}

	if err := conn.WriteJSON(joinMsg); err != nil {
		return fmt.Errorf("failed to send join message: %v", err)
	}

	// Start listening for messages
	go c.listenForMessages()

	return nil
}

func (c *Client) listenForMessages() {
	for {
		var msg WSMessage
		if err := c.conn.ReadJSON(&msg); err != nil {
			log.Printf("Error reading message: %v", err)
			if c.app != nil {
				c.app.QueueUpdateDraw(func() {
					c.textView.Write([]byte(fmt.Sprintf("[red]Connection error: %v[white]\n", err)))
				})
				c.app.Stop()
			}
			return
		}

		switch msg.Type {
		case "join":
			if c.app != nil && c.textView != nil {
				from := msg.From
				c.app.QueueUpdateDraw(func() {
					c.textView.Write([]byte("[green]" + from + " joined the chat[white]\n"))
				})
			}

			// Store the public key
			if msg.PublicKey != "" {
				publicKey, err := crypto.NewKeyFromArmored(msg.PublicKey)
				if err != nil {
					log.Printf("Error parsing public key: %v", err)
					continue
				}
				c.publicKeys[msg.From] = publicKey
			}

			c.updateUsersList()

		case "leave":
			if c.app != nil && c.textView != nil {
				from := msg.From
				c.app.QueueUpdateDraw(func() {
					c.textView.Clear()
					c.textView.Write([]byte("[yellow]Chat cleared[white]\n"))
					c.textView.Write([]byte("[red]" + from + " left the chat[white]\n"))
				})
			}
			delete(c.publicKeys, msg.From)
			c.updateUsersList()

		case "pubkey":
			// Store the public key
			if msg.PublicKey != "" {
				publicKey, err := crypto.NewKeyFromArmored(msg.PublicKey)
				if err != nil {
					log.Printf("Error parsing public key: %v", err)
					continue
				}
				c.publicKeys[msg.From] = publicKey
				c.updateUsersList()
			}

		case "chat":
			// Decrypt the message
			if msg.Payload != "" && msg.From != "" {
				decrypted, err := c.decryptMessage(msg.Payload)
				from := msg.From
				if err != nil {
					if c.app != nil && c.textView != nil {
						c.app.QueueUpdateDraw(func() {
							c.textView.Write([]byte("[red]" + msg.From + " (decryption error)[white]\n"))
						})
					}
					continue
				}

				if c.app != nil && c.textView != nil {
					c.app.QueueUpdateDraw(func() {
						c.textView.Write([]byte("[blue]" + from + ":[white] " + decrypted + "\n"))
					})
				}
			}

		case "clear":
			if c.app != nil && c.textView != nil {
				c.app.QueueUpdateDraw(func() {
					c.textView.Clear()
					c.textView.Write([]byte("[yellow]Chat cleared[white]\n"))
				})
			}
		}
	}
}

func (c *Client) encryptMessage(message string, recipient string) (string, error) {
	recipientKey, exists := c.publicKeys[recipient]
	if !exists {
		return "", fmt.Errorf("no public key for recipient %s", recipient)
	}

	pgp := crypto.PGP()
	encHandle, err := pgp.Encryption().Recipient(recipientKey).New()
	if err != nil {
		return "", fmt.Errorf("failed to create encryption handle: %v", err)
	}

	pgpMessage, err := encHandle.Encrypt([]byte(message))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt message: %v", err)
	}

	armored, err := pgpMessage.ArmorBytes()
	if err != nil {
		return "", fmt.Errorf("failed to armor encrypted message: %v", err)
	}

	return string(armored), nil
}

func (c *Client) decryptMessage(encryptedMessage string) (string, error) {
	pgp := crypto.PGP()
	decHandle, err := pgp.Decryption().DecryptionKey(c.privateKey).New()
	if err != nil {
		return "", fmt.Errorf("failed to create decryption handle: %v", err)
	}

	decrypted, err := decHandle.Decrypt([]byte(encryptedMessage), crypto.Armor)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt message: %v", err)
	}

	return string(decrypted.Bytes()), nil
}

func (c *Client) sendMessage(message string) {
	// Check if UI components are initialized
	if c.app == nil || c.textView == nil {
		log.Printf("UI not initialized properly")
		return
	}

	// Check if message is a command
	if strings.HasPrefix(message, "/") {
		parts := strings.SplitN(message, " ", 2)
		command := parts[0]

		switch command {
		case "/clear":
			msg := WSMessage{Type: "clear"}
			go func() {
				if err := c.conn.WriteJSON(msg); err != nil {
					log.Printf("Error sending clear message: %v", err)
				}
			}()
			return

		case "/help":
			c.textView.Write([]byte("[yellow]Available commands:[white]\n"))
			c.textView.Write([]byte("[yellow]/clear - Clear the chat[white]\n"))
			c.textView.Write([]byte("[yellow]/help - Show this help message[white]\n"))
			return
		}
	}

	// Send message to all users
	if len(c.publicKeys) == 0 {
		c.textView.Write([]byte("[yellow]No other users connected to send messages to[white]\n"))
		return
	}

	// Display the message locally immediately
	c.textView.Write([]byte(fmt.Sprintf("[green]%s:[white] %s\n", c.username, message)))

	// Send encrypted message to each user
	for user := range c.publicKeys {
		if user == c.username {
			continue // Skip sending to self
		}

		// Encrypt the message for this user
		encrypted, err := c.encryptMessage(message, user)
		if err != nil {
			c.textView.Write([]byte(fmt.Sprintf("[red]Error encrypting message for %s: %v[white]\n", user, err)))
			continue
		}

		// Send the encrypted message
		msg := WSMessage{
			Type:    "chat",
			From:    c.username,
			To:      user,
			Payload: encrypted,
		}

		go func(msg WSMessage) {
			if err := c.conn.WriteJSON(msg); err != nil {
				log.Printf("Error sending message to %s: %v", msg.To, err)
			}
		}(msg)
	}
}

func (c *Client) updateUsersList() {
	c.app.QueueUpdateDraw(func() {
		c.usersView.Clear()
		c.usersView.Write([]byte(fmt.Sprintf("[green]%s (you)[white]\n", c.username)))
		for user := range c.publicKeys {
			if user != c.username {
				c.usersView.Write([]byte(fmt.Sprintf("[blue]%s[white]\n", user)))
			}
		}
	})
}

func (c *Client) setupUI() {
	c.app = tview.NewApplication()

	// Create the main flex container
	c.flex = tview.NewFlex().SetDirection(tview.FlexRow)

	// Create a horizontal flex for the chat and users
	contentFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	// Create the text view for displaying messages
	c.textView = tview.NewTextView()
	c.textView.SetDynamicColors(true)
	c.textView.SetRegions(true)
	c.textView.SetScrollable(true)
	c.textView.SetChangedFunc(func() {
		c.textView.ScrollToEnd()
	})
	c.textView.SetBorder(true).SetTitle("Chat")

	// Create the text view for displaying users
	c.usersView = tview.NewTextView()
	c.usersView.SetDynamicColors(true)
	c.usersView.SetBorder(true).SetTitle("Users")

	// Add text views to the content flex
	contentFlex.AddItem(c.textView, 0, 3, true)
	contentFlex.AddItem(c.usersView, 0, 1, false)

	// Create the input field for typing messages
	c.inputField = tview.NewInputField()
	c.inputField.SetLabel("Message: ")
	c.inputField.SetFieldWidth(0)
	c.inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			message := c.inputField.GetText()
			if message != "" {
				c.sendMessage(message)
				c.inputField.SetText("")
			}
		}
	})

	// Add components to the flex container
	c.flex.AddItem(contentFlex, 0, 1, true)
	c.flex.AddItem(c.inputField, 3, 0, false)

	// Set up the UI
	c.app.SetRoot(c.flex, true)
	c.app.SetFocus(c.inputField)
}

func (c *Client) Run() error {
	return c.app.Run()
}

func main() {
	var username string
	if len(os.Args) == 1 {
		fmt.Print("Enter your username: ")
		fmt.Scanln(&username)
	} else {
		username = os.Args[1]
	}

	// Get server URL from environment variable or use default
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "ws://localhost:8080/ws"
	}

	client := NewClient()

	// Setup UI first
	client.setupUI()

	// Then connect to server (which starts the message listener)
	if err := client.connectToServer(serverURL, username); err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	// Finally run the application
	if err := client.Run(); err != nil {
		log.Fatalf("Failed to run client: %v", err)
	}
}
