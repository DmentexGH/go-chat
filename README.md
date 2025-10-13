# Secure Chat Application

A secure, end-to-end encrypted chat application built with Go, featuring a terminal-based UI and WebSocket communication.

![Example Screenshot](example.png)

## Features

- 🔒 **End-to-End Encryption** - Messages are encrypted using PGP before transmission
- 💬 **Real-time Chat** - Instant messaging with WebSocket connections
- 👥 **Multi-user Support** - Connect multiple users to the same chat
- 🖥️ **Terminal UI** - Clean, responsive terminal interface built with tview
- 🐳 **Docker Support** - Easy deployment with Docker
- 🔑 **Automatic Key Management** - PGP key pairs generated and exchanged automatically
- 🗑️ **Ephemeral Communication** - No message storage on server, keys deleted on exit

## Quick Start

### Running Locally

1. **Start the Server:**

   ```bash
   go run ./server/main.go
   ```

2. **Start Clients:**
   ```bash
   go run ./client/main.go
   ```
   Or specify a username:
   ```bash
   go run ./client/main.go username
   ```

### Using Docker

1. **Build and run the server:**

   ```bash
   docker build -t secure-chat-server .
   docker run -p 8080:8080 secure-chat-server
   ```

   The final Docker image is optimized and weighs **~22MB**.

2. **Run clients locally:**
   ```bash
   go run ./client/main.go
   ```

### Configuration

Create a `.env` file to configure the server URL:

```bash
# Copy the example file
cp .env.example .env

# Edit the server URL if needed
# SERVER_URL=ws://your-server.com:8080/ws
```

## How It Works

### Security Architecture

- **End-to-End Encryption**: Messages encrypted client-side, only decrypted by recipients
- **No Server Access**: Server routes encrypted messages but cannot read content
- **No Message Storage**: Server doesn't save messages - all communication is ephemeral
- **Automatic Key Management**: PGP key pairs generated per user, deleted on exit

### Communication Flow

1. **Connection**: Client generates PGP key pair on startup
2. **Key Exchange**: Public keys automatically shared between all users
3. **Messaging**: Messages encrypted for each recipient using their public key
4. **Decryption**: Each user decrypts messages using their private key

## Usage

### Client Interface

The client provides a terminal-based interface with:

- **Chat Area**: Displays messages from all users
- **Users List**: Shows all connected users
- **Input Field**: Type messages to send to all users

### Commands

- `/clear` - Clear the chat history
- `/help` - Show available commands

## Dependencies

- [gin-gonic/gin](https://github.com/gin-gonic/gin) - HTTP web framework
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [rivo/tview](https://github.com/rivo/tview) - Terminal UI library
- [gdamore/tcell](https://github.com/gdamore/tcell) - Terminal cell library
- [ProtonMail/gopenpgp](https://github.com/ProtonMail/gopenpgp) - PGP encryption library

## Development

### Building from Source

```bash
# Build server
cd cmd/server && go build -o ../../server

# Build client
cd cmd/client && go build -o ../../client
```

## Configuration

- **Server Port**: Defaults to 8080 (configurable in server code)
- **WebSocket Endpoint**: `/ws`
- **Client Connection**: `ws://localhost:8080/ws`

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## Acknowledgments

- Built with Go for performance and concurrency
- Uses industry-standard PGP encryption for security (via ProtonMail's gopenpgp)
