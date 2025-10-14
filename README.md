# Secure Chat Application

A secure, end-to-end encrypted chat application built with Go, featuring a terminal-based UI and WebSocket communication.

![Example Screenshot](example.png)

## Features

- 🔒 **End-to-End Encryption** - Messages encrypted client-side using PGP, only decrypted by recipients
- 💬 **Real-time Chat** - Instant messaging with WebSocket connections using gorilla/websocket
- 👥 **Multi-user Support** - Connect multiple users with automatic user discovery and management
- 🖥️ **Terminal UI** - Clean, responsive terminal interface built with tview and tcell
- 🐳 **Docker Support** - Easy deployment with optimized Docker image (~14MB)
- 🔑 **Automatic Key Management** - PGP key pairs generated per user and exchanged automatically
- 🗑️ **Ephemeral Communication** - Server routes encrypted messages only, no storage by design.
- 🚀 **Lightweight Server** - Built with standard net/http package for minimal dependencies
- 📝 **Command Support** - Built-in commands (/clear, /help) for chat management

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

1. **Build and run the server:**

   ```bash
   docker build -t secure-chat-server .
   docker run -p 8080:8080 secure-chat-server
   ```

   The final Docker image is optimized and weighs **~22MB**.

2. **Run clients locally:**
   ```bash
   go run ./client
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

- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [rivo/tview](https://github.com/rivo/tview) - Terminal UI library
- [gdamore/tcell](https://github.com/gdamore/tcell) - Terminal cell library
- [ProtonMail/gopenpgp](https://github.com/ProtonMail/gopenpgp) - PGP encryption library

## Acknowledgments

- Built with Go for performance and concurrency
- Uses standard `net/http` package for lightweight HTTP handling
- Uses industry-standard PGP encryption for security (via ProtonMail's gopenpgp)
