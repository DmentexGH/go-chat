# Secure Chat Application

A secure, end-to-end encrypted chat application built with Go, featuring a terminal-based UI and WebSocket communication.

![Example Screenshot](example.png)

## Features

- 🔒 **End-to-End Encryption** - PGP encryption with automatic key management
- 💬 **Real-time Chat** - WebSocket-based messaging with live user list
- 🖥️ **Terminal UI** - Clean interface built with tview and tcell
- 🐳 **Docker Support** - Optimized deployment (~6MB image)
- � **Lightweight** - Minimal dependencies using standard net/http
- 🗑️ **Ephemeral** - No message storage, server only routes encrypted data

## Quick Start

### Running Locally

1. **Start the Server:**

   ```bash
   go run ./server
   ```

2. **Start Clients:**
   ```bash
   go run ./client
   ```
   Or specify a username:
   ```bash
   go run ./client username
   ```

### Using Docker

```bash
docker build -t secure-chat-server .
docker run -p 8080:8080 secure-chat-server
```

**Image size**: ~6MB

Run clients locally: `go run ./client`

### Configuration

Create a `.env` file to configure the server URL:

```bash
# Copy the example file
cp .env.example .env

# Edit the server URL if needed
# SERVER_URL=ws://your-server.com:8080/ws
```

## How It Works

1. **Connection**: Client generates PGP key pair on startup
2. **Key Exchange**: Public keys automatically shared between users
3. **Messaging**: Messages encrypted for each recipient
4. **Decryption**: Each user decrypts with their private key
5. **Cleanup**: When a user disconnects, their keys are deleted and the chat history is cleared from the UI for everyone

**Security**: Server only routes encrypted messages - cannot read content or store data

## Usage

**Interface**: Chat area, live user list, and input field

**Commands**:

- `/clear` - Clear chat history
- `/help` - Show available commands

## Dependencies

- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [rivo/tview](https://github.com/rivo/tview) - Terminal UI library
- [gdamore/tcell](https://github.com/gdamore/tcell) - Terminal cell library
- [ProtonMail/gopenpgp](https://github.com/ProtonMail/gopenpgp) - PGP encryption library

## Acknowledgments

Built with Go, using standard `net/http` and ProtonMail's gopenpgp for PGP encryption
