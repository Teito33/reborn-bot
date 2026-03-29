package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// SignupEntry represents a user's signup for a role with timestamp
type SignupEntry struct {
	User      *discordgo.User
	Role      string
	Timestamp time.Time
}

// BoostInfo contains information about the boost
type BoostInfo struct {
	Dungeons     int
	KeyLevel     int
	Price        float64
	Note         string
	CutBooster   float64
	CutAdventure float64
	CreatorID    string
	ChannelID    string
}

// SignupSession tracks users who reacted for a specific message
type SignupSession struct {
	MessageID string
	BoostInfo BoostInfo
	Signups   []SignupEntry
	Cancelled bool
	mu        sync.Mutex
}

var (
	signupSessions = make(map[string]*SignupSession)
	sessionMu      sync.Mutex
)

func getOrCreateSession(messageID string) *SignupSession {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	if session, exists := signupSessions[messageID]; exists {
		return session
	}

	session := &SignupSession{
		MessageID: messageID,
		Signups:   make([]SignupEntry, 0),
	}
	signupSessions[messageID] = session
	return session
}

// parseBoostCommand parses the !boost command
// Format: !boost 2x12 140 "note"
func parseBoostCommand(content string) (BoostInfo, error) {
	info := BoostInfo{}

	parts := strings.Fields(content)
	if len(parts) < 3 {
		return info, fmt.Errorf("usage: !boost <dungeons>x<level> <price> [note]")
	}

	// Parse dungeons x level (e.g., 2x12)
	dungeonParts := strings.Split(parts[1], "x")
	if len(dungeonParts) != 2 {
		return info, fmt.Errorf("invalid format for dungeons, use <number>x<level>")
	}

	dungeons, err := strconv.Atoi(dungeonParts[0])
	if err != nil {
		return info, fmt.Errorf("invalid dungeon count: %v", err)
	}

	keyLevel, err := strconv.Atoi(dungeonParts[1])
	if err != nil {
		return info, fmt.Errorf("invalid key level: %v", err)
	}

	price, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return info, fmt.Errorf("invalid price: %v", err)
	}

	// Price is in thousands (k), so multiply by 1000
	price = price * 1000

	// Parse optional note (everything after price, trimming quotes)
	note := ""
	if len(parts) > 3 {
		note = strings.Join(parts[3:], " ")
		note = strings.Trim(note, "\"")
	}

	info.Dungeons = dungeons
	info.KeyLevel = keyLevel
	info.Price = price
	info.Note = note
	info.CutBooster = price * 0.1625
	info.CutAdventure = price * 0.35

	return info, nil
}

// selectBestGroup selects 1 tank, 1 healer, 2 dps from signups in order (FIFO)
// avoiding duplicates (same person can't appear twice)
func selectBestGroup(signups []SignupEntry) (tank *discordgo.User, healer *discordgo.User, dps []*discordgo.User) {
	dps = make([]*discordgo.User, 0)
	selectedUsers := make(map[string]bool)

	for _, signup := range signups {
		userID := signup.User.ID

		// Skip if this user is already selected
		if selectedUsers[userID] {
			continue
		}

		switch signup.Role {
		case "TankR":
			if tank == nil {
				tank = signup.User
				selectedUsers[userID] = true
			}
		case "HealR":
			if healer == nil {
				healer = signup.User
				selectedUsers[userID] = true
			}
		case "DpsR":
			if len(dps) < 2 {
				dps = append(dps, signup.User)
				selectedUsers[userID] = true
			}
		}

		// Check if we have all roles filled
		if tank != nil && healer != nil && len(dps) == 2 {
			break
		}
	}

	return
}

func HandleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("PANIC in HandleMessageCreate: %v", rec)
		}
	}()

	// Ignore bot messages
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Command: !boost - Starts a new signup session
	if strings.HasPrefix(m.Content, "!boost") {
		// Check if user has Management or Advertiser role
		member, err := s.GuildMember(m.GuildID, m.Author.ID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Erreur: impossible de vérifier tes rôles", m.Author.ID))
			return
		}

		hasPermission := false
		for _, roleID := range member.Roles {
			if roleID == "1026892251735015515" || roleID == "1026936430859129013" {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> Seuls les Advertisers et Managers peuvent démarrer un boost.", m.Author.ID))
			return
		}

		boostInfo, err := parseBoostCommand(m.Content)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Error: %s", err.Error()))
			return
		}

		// Create embed message
		embed := &discordgo.MessageEmbed{
			Color: 0x8B0000,
			Image: &discordgo.MessageEmbedImage{
				URL: "https://blizzardwatch.com/wp-content/uploads/2022/02/Dong_Zhuo_Gallywix.jpg",
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "Price:",
					Value:  fmt.Sprintf("%.0fk <:gold:1031594550646550628>", boostInfo.Price/1000),
					Inline: true,
				},
				{
					Name:   "Nb of runs:",
					Value:  fmt.Sprintf("<:transparent:1031668361458892871><:transparent:1031668361458892871> %d", boostInfo.Dungeons),
					Inline: true,
				},
				{
					Name:   "Key level:",
					Value:  fmt.Sprintf("<:transparent:1031668361458892871><:transparent:1031668361458892871> %d", boostInfo.KeyLevel),
					Inline: true,
				},
				{
					Name:   "Booster cut:",
					Value:  fmt.Sprintf("%.0fk <:gold:1031594550646550628>", boostInfo.CutBooster/1000),
					Inline: true,
				},
			},
		}

		if boostInfo.Note != "" {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Note:",
				Value:  boostInfo.Note,
				Inline: false,
			})
		}

		// Send the boost with role mention
		msg, err := s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: "<@&1026942518207729744>",
			Embed:   embed,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Roles: []string{"1026942518207729744"},
			},
		})
		if err != nil {
			log.Printf("error sending message: %v", err)
			return
		}

		// Store boost info in session
		session := getOrCreateSession(msg.ID)
		boostInfo.CreatorID = m.Author.ID
		boostInfo.ChannelID = m.ChannelID
		session.BoostInfo = boostInfo

		// Add reactions to the message
		s.MessageReactionAdd(m.ChannelID, msg.ID, "TankR:1031701109540147211")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "HealR:1031701243342630992")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "DpsR:1031701306475290675")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "crossR:1461783456651415770") // Cancel button
	}
}

func HandleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("PANIC in HandleReactionAdd: %v", rec)
		}
	}()

	// Ignore bot reactions
	if r.UserID == s.State.User.ID {
		return
	}

	// Check if this message has a boost session
	sessionMu.Lock()
	session, exists := signupSessions[r.MessageID]
	sessionMu.Unlock()

	if !exists {
		// Ignore reactions on messages that don't have a boost
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Get the user object
	user, err := s.User(r.UserID)
	if err != nil {
		log.Printf("error getting user: %v", err)
		return
	}

	log.Printf("User %s reacted with emoji Name=%s ID=%s", user.Username, r.Emoji.Name, r.Emoji.ID)

	// Check if cancel emoji (by Name or ID)
	if r.Emoji.Name == "❌" || r.Emoji.Name == "crossR" || r.Emoji.ID == "1461783456651415770" {
		log.Printf("Cancel emoji detected. Creator: %s, UserID: %s", session.BoostInfo.CreatorID, r.UserID)

		// Get guild member to check roles
		member, err := s.GuildMember(r.GuildID, r.UserID)
		hasManagementRole := false

		if err == nil {
			// Check if user has Management role
			for _, roleID := range member.Roles {
				if roleID == "1026892251735015515" {
					hasManagementRole = true
					break
				}
			}
		}

		// Check if user is creator or has Management role
		if r.UserID != session.BoostInfo.CreatorID && !hasManagementRole {
			log.Printf("User %s is not the creator and doesn't have Management role, ignoring cancel", r.UserID)
			// Send error message
			s.ChannelMessageSend(r.ChannelID, fmt.Sprintf("<@%s> Vous n'avez pas l'autorisation d'annuler ce boost", r.UserID))
			return
		}

		if session.Cancelled {
			log.Println("Boost already cancelled")
			return
		}

		log.Println("Cancelling boost...")
		session.Cancelled = true

		// Edit the message to show it's cancelled
		log.Println("About to edit message...")
		cancelEmbed := &discordgo.MessageEmbed{
			Title:       "WoW Boost Signup - ❌ ANNULÉ",
			Color:       0xFF0000,
			Description: "Ce boost a été annulé",
		}

		_, editErr := s.ChannelMessageEditEmbed(r.ChannelID, r.MessageID, cancelEmbed)
		if editErr != nil {
			log.Printf("error editing message: %v", editErr)
		}
		log.Println("Message edited successfully")

		// Get all users who reacted
		log.Println("Building mentions list...")
		var mentions []string
		for _, signup := range session.Signups {
			mentions = append(mentions, fmt.Sprintf("<@%s>", signup.User.ID))
		}

		// Send notification message
		log.Printf("About to send notification with %d mentions...", len(mentions))
		if len(mentions) > 0 {
			notifMessage := fmt.Sprintf("⚠️ **Boost annulé!**\n\n%s\n\nCe boost a été annulé par <@%s>", strings.Join(mentions, " "), r.UserID)
			_, notifErr := s.ChannelMessageSend(r.ChannelID, notifMessage)
			if notifErr != nil {
				log.Printf("error sending notification message: %v", notifErr)
			}
			log.Println("Notification sent successfully")
		}

		log.Println("Cancel operation completed successfully")
		return
	}

	// Don't accept new signups if boost is cancelled
	if session.Cancelled {
		return
	}

	// Add signup to the list
	session.Signups = append(session.Signups, SignupEntry{
		User:      user,
		Role:      r.Emoji.Name,
		Timestamp: time.Now(),
	})

	log.Printf("Total signups: %d", len(session.Signups))
	for i, signup := range session.Signups {
		log.Printf("  [%d] %s - %s", i, signup.User.Username, signup.Role)
	}

	// Try to select a full group
	tank, healer, dps := selectBestGroup(session.Signups)

	log.Printf("Selected - Tank: %v, Healer: %v, DPS: %d", tank != nil, healer != nil, len(dps))

	// Check if we have all roles filled (1 tank, 1 healer, 2 DPS)
	if tank != nil && healer != nil && len(dps) == 2 {
		log.Println("Group complete! Displaying selected players.")
		displaySelectedPlayers(s, r.ChannelID, tank, healer, dps, session.BoostInfo.Note)
		// Clean up the session
		sessionMu.Lock()
		delete(signupSessions, r.MessageID)
		sessionMu.Unlock()
	}
}

func HandleReactionRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
	// Check if this message has a boost session
	sessionMu.Lock()
	session, exists := signupSessions[r.MessageID]
	sessionMu.Unlock()

	if !exists {
		// Ignore reactions on messages that don't have a boost
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Remove the signup entry
	for i, signup := range session.Signups {
		if signup.User.ID == r.UserID && signup.Role == r.Emoji.Name {
			session.Signups = append(session.Signups[:i], session.Signups[i+1:]...)
			break
		}
	}
}

func displaySelectedPlayers(s *discordgo.Session, channelID string, tank *discordgo.User, healer *discordgo.User, dps []*discordgo.User, note string) {
	message := "**Boost Group Selected!**\n\n"

	if tank != nil {
		message += fmt.Sprintf("Tank: <@%s>\n", tank.ID)
	}

	if healer != nil {
		message += fmt.Sprintf("Healer: <@%s>\n", healer.ID)
	}

	if len(dps) > 0 {
		message += "DPS: "
		for i, dpsPlayer := range dps {
			if i > 0 {
				message += ", "
			}
			message += fmt.Sprintf("<@%s>", dpsPlayer.ID)
		}
		message += "\n"
	}

	if note != "" {
		message += fmt.Sprintf("\n**Note:** %s", note)
	}

	_, err := s.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("error sending selected players message: %v", err)
	}
}
