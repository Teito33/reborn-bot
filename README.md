# Discord Bot for Player Signup

This project is a Discord bot written in Go that allows users to sign up for roles in a game by reacting with specific emojis. The bot selects one tank, one healer, and two DPS players from the first responders and displays their names.

## Project Structure

```
discord-bot
├── src
│   ├── main.go          # Entry point of the bot application
│   ├── handlers
│   │   └── reaction.go  # Logic for handling emoji reactions
│   ├── commands
│   │   └── signup.go    # Command for users to sign up
│   ├── services
│   │   └── selection.go  # Selection logic for choosing players
│   └── models
│       └── player.go    # Player struct definition
├── go.mod                # Module definition for the Go project
├── go.sum                # Checksums for module dependencies
└── README.md             # Documentation for the project
```

## Setup Instructions

1. **Clone the repository:**
   ```
   git clone <repository-url>
   cd discord-bot
   ```

2. **Install dependencies:**
   Ensure you have Go installed, then run:
   ```
   go mod tidy
   ```

3. **Create a Discord bot application:**
   - Go to the Discord Developer Portal.
   - Create a new application and add a bot to it.
   - Copy the bot token.

4. **Set up environment variables:**
   Create a `.env` file in the `src` directory and add your bot token:
   ```
   DISCORD_TOKEN=your_bot_token_here
   ```

5. **Run the bot:**
   ```
   go run src/main.go
   ```

## Usage

- Users can sign up for roles by reacting with the following emojis:
  - 🛡️ for Tank
  - ❤️ for Healer
  - ⚔️ for DPS

- The bot will select one tank, one healer, and two DPS players from the reactions and announce the selected players in the channel.

## Contributing

Feel free to submit issues or pull requests for improvements or bug fixes.