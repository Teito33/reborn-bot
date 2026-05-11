package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
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
	BoostType    string // "m+" or "leveling"
	Dungeons     int    // For M+ boosts
	KeyLevel     int    // For M+ boosts
	StartLevel   int    // For leveling boosts
	EndLevel     int    // For leveling boosts
	Price        float64
	Note         string
	CutBooster   float64
	CutAdventure float64
	CreatorID    string
	ChannelID    string
	KeysRequired int // Only for M+ boosts
}

// KeystoneEntry represents a user's keystones
type KeystoneEntry struct {
	UserID string
	Count  int
}

// SignupSession tracks users who reacted for a specific message
type SignupSession struct {
	MessageID string
	BoostInfo BoostInfo
	Signups   []SignupEntry
	Keystones []KeystoneEntry
	Cancelled bool
	mu        sync.Mutex
}

var (
	signupSessions = make(map[string]*SignupSession)
	sessionMu      sync.Mutex
	lastBoostID    string // Track the last boost message ID
	lastBoostMu    sync.Mutex
	dataFile       = "boosts_data.json"
)

// PersistedSession represents a boost session saved to disk
type PersistedSession struct {
	MessageID string          `json:"messageID"`
	BoostInfo BoostInfo       `json:"boostInfo"`
	Signups   []SignupEntry   `json:"signups"`
	Keystones []KeystoneEntry `json:"keystones"`
	Cancelled bool            `json:"cancelled"`
}

// LoadSessions loads all saved sessions from disk
func LoadSessions() error {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("No saved sessions found, starting fresh")
			return nil
		}
		return err
	}

	var persisted []PersistedSession
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	sessionMu.Lock()
	defer sessionMu.Unlock()

	for _, p := range persisted {
		// Ensure BoostType is set for old sessions
		if p.BoostInfo.BoostType == "" {
			p.BoostInfo.BoostType = "m+"
		}

		session := &SignupSession{
			MessageID: p.MessageID,
			BoostInfo: p.BoostInfo,
			Signups:   p.Signups,
			Keystones: p.Keystones,
			Cancelled: p.Cancelled,
		}
		signupSessions[p.MessageID] = session

		// Update lastBoostID if this is more recent
		if lastBoostID == "" {
			lastBoostID = p.MessageID
		}
	}

	log.Printf("Loaded %d sessions from disk", len(persisted))
	return nil
}

// SaveSessions saves all sessions to disk
func SaveSessions() error {
	sessionMu.Lock()
	defer sessionMu.Unlock()

	var persisted []PersistedSession
	for _, session := range signupSessions {
		p := PersistedSession{
			MessageID: session.MessageID,
			BoostInfo: session.BoostInfo,
			Signups:   session.Signups,
			Keystones: session.Keystones,
			Cancelled: session.Cancelled,
		}
		persisted = append(persisted, p)
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(dataFile, data, 0644)
}

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
// Format: !boost 2x12 140 k2 "note"
// k2 = 2 keystones required (optional)
func parseBoostCommand(content string) (BoostInfo, error) {
	info := BoostInfo{KeysRequired: 0, BoostType: "m+"}

	parts := strings.Fields(content)
	if len(parts) < 3 {
		return info, fmt.Errorf("usage: !boost <dungeons>x<level> <price> [k<number>] [note]")
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

	// Parse price - remove trailing 'k' if present
	priceStr := parts[2]
	if strings.HasSuffix(strings.ToLower(priceStr), "k") {
		priceStr = priceStr[:len(priceStr)-1]
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return info, fmt.Errorf("invalid price: %v", err)
	}

	// Price is in thousands (k), so multiply by 1000
	price = price * 1000

	// Parse optional keystones and note
	note := ""
	keysRequired := 0

	for i := 3; i < len(parts); i++ {
		part := parts[i]

		// Check if this is a keystones requirement (e.g., k2, k3)
		if strings.HasPrefix(part, "k") && len(part) > 1 {
			keys, err := strconv.Atoi(part[1:])
			if err == nil && keys > 0 {
				keysRequired = keys
				continue
			}
		}

		// Everything else is the note
		note = strings.Join(parts[i:], " ")
		note = strings.Trim(note, "\"")
		break
	}

	info.Dungeons = dungeons
	info.KeyLevel = keyLevel
	info.Price = price
	info.Note = note
	info.KeysRequired = keysRequired
	info.BoostType = "m+"
	info.CutBooster = price * 0.1625
	info.CutAdventure = price * 0.35

	return info, nil
}

// parseLevelingCommand parses the !plvl command
// Format: !plvl 1-90 190 or !plvl 1-90 190k Ceci est une note
func parseLevelingCommand(content string) (BoostInfo, error) {
	info := BoostInfo{BoostType: "leveling"}

	parts := strings.Fields(content)
	if len(parts) < 3 {
		return info, fmt.Errorf("usage: !plvl <startLevel>-<endLevel> <price> [note]")
	}

	// Parse levels (e.g., 80-90)
	levelParts := strings.Split(parts[1], "-")
	if len(levelParts) != 2 {
		return info, fmt.Errorf("invalid format for levels, use <startLevel>-<endLevel>")
	}

	startLevel, err := strconv.Atoi(levelParts[0])
	if err != nil {
		return info, fmt.Errorf("invalid start level: %v", err)
	}

	endLevel, err := strconv.Atoi(levelParts[1])
	if err != nil {
		return info, fmt.Errorf("invalid end level: %v", err)
	}

	// Validate level ranges
	// Start level: 1-89
	// End level: 60-90
	if startLevel < 1 || startLevel > 89 {
		return info, fmt.Errorf("start level must be between 1 and 89")
	}
	if endLevel < 60 || endLevel > 90 {
		return info, fmt.Errorf("end level must be between 60 and 90")
	}
	if startLevel >= endLevel {
		return info, fmt.Errorf("start level must be less than end level")
	}

	// Parse price - remove trailing 'k' if present
	priceStr := parts[2]
	if strings.HasSuffix(strings.ToLower(priceStr), "k") {
		priceStr = priceStr[:len(priceStr)-1]
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return info, fmt.Errorf("invalid price: %v", err)
	}

	// Price is in thousands (k), so multiply by 1000
	price = price * 1000

	// Parse optional note
	note := ""
	if len(parts) > 3 {
		note = strings.Join(parts[3:], " ")
		note = strings.Trim(note, "\"")
	}

	info.StartLevel = startLevel
	info.EndLevel = endLevel
	info.Price = price
	info.Note = note
	// For leveling: 30% advertiser, 70% booster
	info.CutBooster = price * 0.70
	info.CutAdventure = price * 0.30

	return info, nil
}

// selectBestGroup selects 1 tank, 1 healer, 2 dps from signups in order (FIFO)
// Prioritizes users who have clicked on keystoneR emoji
// First verifies that a complete group can be formed, then selects based on role priority
// Role priority: Tank > Healer > DPS
func selectBestGroup(signups []SignupEntry, keystoneUsers map[string]bool) (tank *discordgo.User, healer *discordgo.User, dps []*discordgo.User) {
	dps = make([]*discordgo.User, 0)

	// Build a map of all roles clicked by each user
	userRoles := make(map[string]map[string]bool)
	userObjects := make(map[string]*discordgo.User)

	for _, signup := range signups {
		userID := signup.User.ID
		if userRoles[userID] == nil {
			userRoles[userID] = make(map[string]bool)
			userObjects[userID] = signup.User
		}
		userRoles[userID][signup.Role] = true
	}

	// Helper function to verify if we can form a complete group from available users
	canFormGroup := func(keystoneOnly bool) bool {
		var hasTank, hasHeal bool
		dpsCount := 0

		for userID, roles := range userRoles {
			// Filter by keystone status if needed
			if keystoneOnly && !keystoneUsers[userID] {
				continue
			}
			if !keystoneOnly && keystoneUsers[userID] {
				continue
			}

			if roles["TankR"] {
				hasTank = true
			}
			if roles["HealR"] {
				hasHeal = true
			}
			if roles["DpsR"] {
				dpsCount++
			}
		}

		return hasTank && hasHeal && dpsCount >= 2
	}

	// Helper function to select users for a group using constraint-first algorithm
	selectUsersFromGroup := func(keystoneOnly bool) {
		selectedUsers := make(map[string]bool)

		// Collect unique users in FIFO order and separate by flexibility
		seenUsers := make(map[string]bool)
		var constrainedUsers []string  // Users who can only do ONE role
		var flexibleUsers []string     // Users who can do MULTIPLE roles

		for _, signup := range signups {
			userID := signup.User.ID
			if seenUsers[userID] {
				continue
			}
			seenUsers[userID] = true

			// Filter by keystone status if needed
			if keystoneOnly && !keystoneUsers[userID] {
				continue
			}
			if !keystoneOnly && keystoneUsers[userID] {
				continue
			}

			// Count how many roles this user can do
			rolesAvailable := 0
			if userRoles[userID]["TankR"] {
				rolesAvailable++
			}
			if userRoles[userID]["HealR"] {
				rolesAvailable++
			}
			if userRoles[userID]["DpsR"] {
				rolesAvailable++
			}

			if rolesAvailable == 1 {
				constrainedUsers = append(constrainedUsers, userID)
			} else if rolesAvailable > 1 {
				flexibleUsers = append(flexibleUsers, userID)
			}
		}

		// First: select constrained users (those who can only do ONE role)
		for _, userID := range constrainedUsers {
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}

			// Early exit if all roles filled
			if tank != nil && healer != nil && len(dps) == 2 {
				return
			}
		}

		// Second: select flexible users (those who can do MULTIPLE roles) with priority Tank > Heal > DPS
		for _, userID := range flexibleUsers {
			if selectedUsers[userID] {
				continue
			}

			// Assign based on priority and availability
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}

			// Early exit if all roles filled
			if tank != nil && healer != nil && len(dps) == 2 {
				return
			}
		}
	}

	// First pass: check if keystoneUsers can form a complete group
	if canFormGroup(true) {
		selectUsersFromGroup(true)
		return
	}

	// Second pass: check if combining keystoneUsers + non-keystoneUsers can form a group
	if canFormGroup(false) {
		selectedUsers := make(map[string]bool)

		// First select from keystoneUsers
		seenUsers := make(map[string]bool)
		var constrainedKeystones []string
		var flexibleKeystones []string

		for _, signup := range signups {
			userID := signup.User.ID
			if !keystoneUsers[userID] || seenUsers[userID] {
				continue
			}
			seenUsers[userID] = true

			rolesCount := 0
			if userRoles[userID]["TankR"] {
				rolesCount++
			}
			if userRoles[userID]["HealR"] {
				rolesCount++
			}
			if userRoles[userID]["DpsR"] {
				rolesCount++
			}

			if rolesCount == 1 {
				constrainedKeystones = append(constrainedKeystones, userID)
			} else {
				flexibleKeystones = append(flexibleKeystones, userID)
			}
		}

		// Select constrained keystoneUsers first
		for _, userID := range constrainedKeystones {
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}
		}

		// Then select flexible keystoneUsers
		for _, userID := range flexibleKeystones {
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}

			if tank != nil && healer != nil && len(dps) == 2 {
				return
			}
		}

		// If still not complete, select from non-keystoneUsers
		seenUsers = make(map[string]bool)
		var constrainedNonKeystones []string
		var flexibleNonKeystones []string

		for _, signup := range signups {
			userID := signup.User.ID
			if keystoneUsers[userID] || seenUsers[userID] || selectedUsers[userID] {
				continue
			}
			seenUsers[userID] = true

			rolesCount := 0
			if userRoles[userID]["TankR"] {
				rolesCount++
			}
			if userRoles[userID]["HealR"] {
				rolesCount++
			}
			if userRoles[userID]["DpsR"] {
				rolesCount++
			}

			if rolesCount == 1 {
				constrainedNonKeystones = append(constrainedNonKeystones, userID)
			} else {
				flexibleNonKeystones = append(flexibleNonKeystones, userID)
			}
		}

		// Select constrained non-keystoneUsers
		for _, userID := range constrainedNonKeystones {
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}
		}

		// Finally select flexible non-keystoneUsers
		for _, userID := range flexibleNonKeystones {
			if tank == nil && userRoles[userID]["TankR"] {
				tank = userObjects[userID]
				selectedUsers[userID] = true
			} else if healer == nil && userRoles[userID]["HealR"] {
				healer = userObjects[userID]
				selectedUsers[userID] = true
			} else if len(dps) < 2 && userRoles[userID]["DpsR"] {
				dps = append(dps, userObjects[userID])
				selectedUsers[userID] = true
			}

			if tank != nil && healer != nil && len(dps) == 2 {
				return
			}
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

	// Command: !list - Show signup order for last boost
	if strings.HasPrefix(m.Content, "!list") {
		// Check if user has Advertiser or Management role
		member, err := s.GuildMember(m.GuildID, m.Author.ID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> Erreur: impossible de vérifier tes rôles", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> Seuls les Advertisers et Managers peuvent utiliser cette commande.", m.Author.ID))
			return
		}

		// Get the last boost session
		lastBoostMu.Lock()
		boostID := lastBoostID
		lastBoostMu.Unlock()

		if boostID == "" {
			s.ChannelMessageSend(m.ChannelID, "❌ Aucun boost trouvé. Créez-en un avec `!boost`")
			return
		}

		sessionMu.Lock()
		session, exists := signupSessions[boostID]
		sessionMu.Unlock()

		if !exists {
			s.ChannelMessageSend(m.ChannelID, "❌ La session du dernier boost n'existe plus.")
			return
		}

		session.mu.Lock()
		defer session.mu.Unlock()

		displayTagList(s, m.ChannelID, session)
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

		// Add keystones field if required
		if boostInfo.KeysRequired > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Keystones needed:",
				Value:  fmt.Sprintf("🔑 %d", boostInfo.KeysRequired),
				Inline: true,
			})
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

		// Save as last boost
		lastBoostMu.Lock()
		lastBoostID = msg.ID
		lastBoostMu.Unlock()

		// Add reactions to the message
		s.MessageReactionAdd(m.ChannelID, msg.ID, "TankR:1031701109540147211")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "HealR:1031701243342630992")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "DpsR:1031701306475290675")

		// Add Keystone reaction if keystones are required
		if boostInfo.KeysRequired > 0 {
			s.MessageReactionAdd(m.ChannelID, msg.ID, "KeystoneR:1502706534231052458")
		}

		// Add cancel button last
		s.MessageReactionAdd(m.ChannelID, msg.ID, "crossR:1461783456651415770")

		// Save sessions to disk
		SaveSessions()
	}

	// Command: !plvl - Starts a new leveling boost session
	if strings.HasPrefix(m.Content, "!plvl") {
		// Check if user has Management or Advertiser role
		member, err := s.GuildMember(m.GuildID, m.Author.ID)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Erreur: impossible de vérifier tes rôles"))
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

		boostInfo, err := parseLevelingCommand(m.Content)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ Error: %s", err.Error()))
			return
		}

		// Create embed message for leveling boost
		embed := &discordgo.MessageEmbed{
			Color: 0x00AA00, // Green for leveling
			Title: fmt.Sprintf("WoW Leveling Boost %d-%d", boostInfo.StartLevel, boostInfo.EndLevel),
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

		// Send the leveling boost with role mention
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

		// Save as last boost
		lastBoostMu.Lock()
		lastBoostID = msg.ID
		lastBoostMu.Unlock()

		// Add only the mushlvl reaction for leveling
		s.MessageReactionAdd(m.ChannelID, msg.ID, "mushlvl:1502706362528960623")
		s.MessageReactionAdd(m.ChannelID, msg.ID, "crossR:1461783456651415770") // Cancel button

		// Save sessions to disk
		SaveSessions()
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

	// Handle leveling boost - only one reaction needed for mushlvl
	if session.BoostInfo.BoostType == "leveling" {
		// Check if cancel emoji
		if r.Emoji.Name == "❌" || r.Emoji.Name == "crossR" || r.Emoji.ID == "1461783456651415770" {
			// Handle cancellation (same as for M+)
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
				s.ChannelMessageSend(r.ChannelID, fmt.Sprintf("<@%s> Vous n'avez pas l'autorisation d'annuler ce boost", r.UserID))
				return
			}

			if session.Cancelled {
				log.Println("Boost already cancelled")
				return
			}

			log.Println("Cancelling leveling boost...")
			session.Cancelled = true

			// Edit the message to show it's cancelled
			cancelEmbed := &discordgo.MessageEmbed{
				Title:       "WoW Leveling Boost - ❌ ANNULÉ",
				Color:       0xFF0000,
				Description: "Ce boost a été annulé",
			}

			_, editErr := s.ChannelMessageEditEmbed(r.ChannelID, r.MessageID, cancelEmbed)
			if editErr != nil {
				log.Printf("error editing message: %v", editErr)
			}

			// Get all users who reacted (without duplicates)
			mentionedUsers := make(map[string]bool)
			var mentions []string
			for _, signup := range session.Signups {
				if !mentionedUsers[signup.User.ID] {
					mentions = append(mentions, fmt.Sprintf("<@%s>", signup.User.ID))
					mentionedUsers[signup.User.ID] = true
				}
			}

			if len(mentions) > 0 {
				notifMessage := fmt.Sprintf("⚠️ **Boost annulé!**\n\n%s\n\nCe boost a été annulé par <@%s>", strings.Join(mentions, " "), r.UserID)
				s.ChannelMessageSend(r.ChannelID, notifMessage)
				SaveSessions()
			}
			return
		}

		// Check if mushlvl emoji (leveling boost reaction)
		if r.Emoji.Name == "mushlvl" || r.Emoji.ID == "1502706362528960623" {
			// For leveling boost, immediately display selected player and launch
			displaySelectedPlayersLeveling(s, r.ChannelID, user, session.BoostInfo.Note, session.BoostInfo.CreatorID)

			// Clean up the session
			sessionMu.Lock()
			delete(signupSessions, r.MessageID)
			sessionMu.Unlock()

			// Save sessions to disk
			SaveSessions()
		}
		return
	}

	// Check if keystone emoji (by Name or ID) - only for M+ boosts
	if (r.Emoji.Name == "KeystoneR" || r.Emoji.ID == "1502706534231052458") && session.BoostInfo.BoostType == "m+" {
		// Check if keystones are required for this boost
		if session.BoostInfo.KeysRequired <= 0 {
			return
		}

		// Add or update keystone entry
		found := false
		for i, ks := range session.Keystones {
			if ks.UserID == r.UserID {
				session.Keystones[i].Count++
				found = true
				break
			}
		}
		if !found {
			session.Keystones = append(session.Keystones, KeystoneEntry{
				UserID: r.UserID,
				Count:  1,
			})
		}

		log.Printf("Keystone added for user %s. Total keystones: %d (Required: %d)", user.Username, len(session.Keystones), session.BoostInfo.KeysRequired)

		// Check if we now have enough keystones to launch the boost
		keystoneUsers := make(map[string]bool)
		for _, ks := range session.Keystones {
			keystoneUsers[ks.UserID] = true
		}

		tank, healer, dps := selectBestGroup(session.Signups, keystoneUsers)
		hasAllRoles := tank != nil && healer != nil && len(dps) == 2
		hasEnoughKeystones := session.BoostInfo.KeysRequired <= 0 || len(session.Keystones) >= session.BoostInfo.KeysRequired

		log.Printf("After keystone - AllRoles: %v, EnoughKeystones: %v (Required: %d, Have: %d)",
			hasAllRoles, hasEnoughKeystones, session.BoostInfo.KeysRequired, len(session.Keystones))

		if hasAllRoles && hasEnoughKeystones {
			log.Println("Group complete! Displaying selected players.")
			displaySelectedPlayers(s, r.ChannelID, tank, healer, dps, session.BoostInfo.Note, session.BoostInfo.KeysRequired, len(session.Keystones), session.BoostInfo.CreatorID)
			// Clean up the session
			sessionMu.Lock()
			delete(signupSessions, r.MessageID)
			sessionMu.Unlock()
			// Save sessions to disk
			SaveSessions()
		} else {
			// Save sessions to disk even if not complete
			SaveSessions()
		}
		return
	}

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

		// Get all users who reacted (without duplicates)
		log.Println("Building mentions list...")
		mentionedUsers := make(map[string]bool)
		var mentions []string
		for _, signup := range session.Signups {
			if !mentionedUsers[signup.User.ID] {
				mentions = append(mentions, fmt.Sprintf("<@%s>", signup.User.ID))
				mentionedUsers[signup.User.ID] = true
			}
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
			// Save sessions to disk
			SaveSessions()
		}

		log.Println("Cancel operation completed successfully")
		return
	}

	// Don't accept new signups if boost is cancelled
	if session.Cancelled {
		return
	}

	// For M+ boosts, accept TankR/HealR/DpsR reactions
	if session.BoostInfo.BoostType == "m+" {
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

		// Create a map of keystoneUsers for priority selection
		keystoneUsers := make(map[string]bool)
		for _, ks := range session.Keystones {
			keystoneUsers[ks.UserID] = true
		}

		// Try to select a full group (prioritizing keystoneUsers)
		tank, healer, dps := selectBestGroup(session.Signups, keystoneUsers)

		log.Printf("Selected - Tank: %v, Healer: %v, DPS: %d", tank != nil, healer != nil, len(dps))

		// Check if we have all roles filled (1 tank, 1 healer, 2 DPS) and keystones if required
		hasAllRoles := tank != nil && healer != nil && len(dps) == 2
		hasEnoughKeystones := session.BoostInfo.KeysRequired <= 0 || len(session.Keystones) >= session.BoostInfo.KeysRequired

		log.Printf("Boost check - AllRoles: %v, EnoughKeystones: %v (Required: %d, Have: %d)",
			hasAllRoles, hasEnoughKeystones, session.BoostInfo.KeysRequired, len(session.Keystones))

		if hasAllRoles && hasEnoughKeystones {
			log.Println("Group complete! Displaying selected players.")
			displaySelectedPlayers(s, r.ChannelID, tank, healer, dps, session.BoostInfo.Note, session.BoostInfo.KeysRequired, len(session.Keystones), session.BoostInfo.CreatorID)
			// Clean up the session
			sessionMu.Lock()
			delete(signupSessions, r.MessageID)
			sessionMu.Unlock()
			// Save sessions to disk
			SaveSessions()
		} else {
			// Save sessions to disk even if not complete
			SaveSessions()
		}
	}
}

func displaySelectedPlayersLeveling(s *discordgo.Session, channelID string, booster *discordgo.User, note string, creatorID string) {
	message := fmt.Sprintf("<@%s>\n\n**Leveling Boost Started!**\n\n", creatorID)
	message += fmt.Sprintf("Booster: <@%s>\n", booster.ID)

	if note != "" {
		message += fmt.Sprintf("\n**Note:** %s", note)
	}

	_, err := s.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("error sending selected players message: %v", err)
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

	// Check if this is a keystone removal
	if r.Emoji.Name == "KeystoneR" || r.Emoji.ID == "1502706534231052458" {
		// Remove the keystone entry
		for i, ks := range session.Keystones {
			if ks.UserID == r.UserID {
				session.Keystones = append(session.Keystones[:i], session.Keystones[i+1:]...)
				break
			}
		}
		return
	}

	// Remove the signup entry
	for i, signup := range session.Signups {
		if signup.User.ID == r.UserID && signup.Role == r.Emoji.Name {
			session.Signups = append(session.Signups[:i], session.Signups[i+1:]...)
			break
		}
	}
}

func displaySelectedPlayers(s *discordgo.Session, channelID string, tank *discordgo.User, healer *discordgo.User, dps []*discordgo.User, note string, keysRequired int, keystoneCount int, creatorID string) {
	message := fmt.Sprintf("<@%s>\n\n**Boost Group Selected!**\n\n", creatorID)

	if tank != nil {
		message += fmt.Sprintf("<:TankR:1031701109540147211> <@%s>\n", tank.ID)
	}

	if healer != nil {
		message += fmt.Sprintf("<:HealR:1031701243342630992> <@%s>\n", healer.ID)
	}

	if len(dps) > 0 {
		message += "<:DpsR:1031701306475290675> "
		for i, dpsPlayer := range dps {
			if i > 0 {
				message += ", "
			}
			message += fmt.Sprintf("<@%s>", dpsPlayer.ID)
		}
		message += "\n"
	}

	if keysRequired > 0 {
		message += fmt.Sprintf("\n🔑 **Keystones:** %d/%d\n", keystoneCount, keysRequired)
	}

	if note != "" {
		message += fmt.Sprintf("\n**Note:** %s", note)
	}

	_, err := s.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("error sending selected players message: %v", err)
	}
}

func displayTagList(s *discordgo.Session, channelID string, session *SignupSession) {
	message := "**📋 Signup Order - Tag List**\n\n"

	if len(session.Signups) == 0 {
		message += "Aucun signup pour ce boost."
	} else {
		// Display all signups with timestamps (including duplicates)
		counter := 1
		for _, signup := range session.Signups {
			timeStr := signup.Timestamp.Format("15:04:05")
			message += fmt.Sprintf("%d. <@%s> - **%s** (%s)\n", counter, signup.User.ID, signup.Role, timeStr)
			counter++
		}
	}

	if len(session.Keystones) > 0 {
		message += "\n**🔑 Keystones:**\n"
		for i, ks := range session.Keystones {
			message += fmt.Sprintf("%d. <@%s>\n", i+1, ks.UserID)
		}
	}

	_, err := s.ChannelMessageSend(channelID, message)
	if err != nil {
		log.Printf("error sending tag list message: %v", err)
	}
}
