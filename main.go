package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Answers struct {
	OriginChannelID string
	FavFood         string
	FavGane         string
	RecordID        int64
}

func (a *Answers) ToMessageEmbed() *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{
			Name:  "Favorite Food",
			Value: a.FavFood,
		},
		{
			Name:  "Favorite Game",
			Value: a.FavGane,
		},
		{
			Name:  "Record ID",
			Value: strconv.FormatInt(a.RecordID, 10),
		},
	}
	return &discordgo.MessageEmbed{
		Title:  "New response!",
		Fields: fields,
	}
}

var (
	response map[string]Answers = make(map[string]Answers)
	db       *sql.DB
)

const prefix = "!gobot"

func main() {

	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		slog.Error(err.Error())
		return
	}

	db, err = sql.Open("mysql", os.Getenv("DB_USER")+":"+os.Getenv("DB_PASSWORD")+"@tcp("+os.Getenv("DB_HOST")+":"+os.Getenv("DB_PORT")+")/"+os.Getenv("DB_NAME"))
	if err != nil {
		slog.Error(err.Error())
		return
	}

	defer db.Close()

	dg.Identify.Intents = discordgo.IntentsAllWithoutPrivileged

	dg.AddHandler(onMessageCreate)
	dg.AddHandler(reactionAddHandler)
	dg.AddHandler(reactionRemoveHandler)

	err = dg.Open()
	if err != nil {
		slog.Error(err.Error())
		return
	}

	slog.Info("Bot is running. Press CTRL+C to exit.\n")
	sc := make(chan os.Signal, 1)

	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close down the Discord session.
	dg.Close()
}

func init() {
	buildInfo, _ := debug.ReadBuildInfo()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})).WithGroup("program_info").With(slog.Int("pid", os.Getpid()), slog.String("go_version", buildInfo.GoVersion))

	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Error(err.Error())
	}
}

func reactionRemoveHandler(s *discordgo.Session, m *discordgo.MessageReactionRemove) {
	slog.Info(fmt.Sprintf("%s reacted with %s", s.State.User.ID, m.Emoji.Name))
	if m.Emoji.Name == "👍" {
		s.GuildMemberRoleRemove(m.GuildID, m.UserID, os.Getenv("ROLE_ID"))
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s has been removed from %v", m.UserID, m.Emoji.Name))
	}
}

func reactionAddHandler(s *discordgo.Session, m *discordgo.MessageReactionAdd) {
	slog.Info(fmt.Sprintf("%s reacted with %s", s.State.User.ID, m.Emoji.Name))
	if m.Emoji.Name == "👍" {
		s.GuildMemberRoleAdd(m.GuildID, m.UserID, os.Getenv("ROLE_ID"))
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s has been added to %v", m.UserID, m.Emoji.Name))
	}
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.GuildID == "" {
		promptResponseHandler(db, s, m)
	}

	args := strings.Split(m.Content, " ")
	if args[0] == prefix {
		if args[1] == "hello" {
			helloHandler(s, m)
		}

		if args[1] == "proverbs" {
			proverbHandler(s, m)
		}

		if args[1] == "prompt" {
			promptHandler(s, m)
		}

		if args[1] == "answer" {
			answerHandler(db, s, m)
		}
	}

	slog.Info(fmt.Sprintf("Message from %s: %s", m.Author.Username, m.Content))
}

func answerHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {

	sql := strings.Split(m.Content, " ")
	if len(sql) < 3 {
		s.ChannelMessageSend(m.ChannelID, "an ID must be provided. Example: !gobot answer 1")
		return
	}

	id, err := strconv.Atoi(sql[2])
	if err != nil {
		slog.Error(err.Error())
		return
	}

	var (
		recordID  int64
		answerStr string
		userID    int64

		answer Answers
	)

	query := "SELECT id, payload, user_id FROM discord_messages WHERE id = ?"
	row := db.QueryRow(query, id)
	err = row.Scan(&recordID, &answerStr, &userID)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	err = json.Unmarshal([]byte(answerStr), &answer)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	answer.RecordID = recordID
	embed := answer.ToMessageEmbed()
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}

func promptResponseHandler(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {
	answer, ok := response[m.ChannelID]
	if !ok {
		return
	}

	if answer.FavFood == "" {
		answer.FavFood = m.Content

		s.ChannelMessageSend(m.ChannelID, "Great! What is your favorite game now?")
		response[m.ChannelID] = answer
		return
	} else {
		answer.FavGane = m.Content
		query := "INSERT INTO discord_messages (payload, user_id) VALUES (?, ?)"
		jbytes, err := json.Marshal(answer)
		if err != nil {
			slog.Error(err.Error())
			return
		}

		res, err := db.Exec(query, string(jbytes), m.ChannelID)
		if err != nil {
			slog.Error(err.Error())
			return
		}

		lastInserted, err := res.LastInsertId()
		if err != nil {
			slog.Error(err.Error())
			return
		}

		answer.RecordID = lastInserted

		embed := answer.ToMessageEmbed()
		s.ChannelMessageSendEmbed(answer.OriginChannelID, embed)

		delete(response, m.ChannelID)
	}
}

func promptHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		slog.Error(err.Error())
		return
	}

	if _, ok := response[channel.ID]; !ok {
		response[channel.ID] = Answers{
			OriginChannelID: m.ChannelID,
			FavFood:         "",
			FavGane:         "",
		}

		s.ChannelMessageSend(channel.ID, "Hey there! Here are some questions")
		s.ChannelMessageSend(channel.ID, "What is your favorite food?")
	} else {
		s.ChannelMessageSend(m.ChannelID, "We're still waiting... 😅")
	}

}

func helloHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Hi there :wave: %s", m.Author.Username))
}

func proverbHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// 20 english proverbs
	proverbs := []string{
		"Actions speak louder than words.",
		"The pen is mightier than the sword.",
		"When in Rome, do as the Romans do.",
		"The squeaky wheel gets the grease.",
		"When the going gets tough, the tough get going.",
		"Fortune favors the bold.",
		"People who live in glass houses should not throw stones.",
		"Better late than never.",
		"Two wrongs don't make a right.",
		"The early bird catches the worm.",
		"When in Rome, do as the Romans do.",
		"Where there's smoke, there's fire.",
		"Hope for the best, but prepare for the worst.",
		"Better safe than sorry.",
		"Keep your friends close and your enemies closer.",
		"A picture is worth a thousand words.",
		"Beauty is in the eye of the beholder.",
		"Necessity is the mother of invention.",
		"Discretion is the greater part of valor.",
		"Rome wasn't built in a day.",
	}

	_ = rand.Intn(len(proverbs))

	author := &discordgo.MessageEmbedAuthor{
		Name:    "Rob pike",
		IconURL: "https://avatars.githubusercontent.com/u/343043?v=4",
		URL:     "https://go-proverbs.github.io/",
	}

	embed := &discordgo.MessageEmbed{
		Title:  "Random Proverb",
		Author: author,
	}

	// s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Here is a random proverb: %s", proverbs[selection]))
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
