package telegram

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/pyuldashev912/tracker/internal/client"
	"github.com/pyuldashev912/tracker/internal/events"
	"github.com/pyuldashev912/tracker/internal/storage"
	"github.com/pyuldashev912/tracker/pkg/e"
)

const (
	addCmdA    = "/add"
	addCmdB    = "➕ Add"
	updCmdA    = "/upd"
	updCmdB    = "🔄 Update"
	listCmdA   = "/list"
	listCmdB   = "📝 List"
	cancelCmdA = "/cancel"
	cancelCmdB = "❌ Cancel"

	startCmd = "/start"
	helpCmd  = "/help"
)

var (
	ErrInvalidInput = errors.New("invalid input")
)

func (p *Processor) doCommand(event *events.Event, meta *events.Meta) error {
	text := strings.TrimSpace(event.Text)

	log.Printf("got new command '%s' from '%s'", text, event.Username)

	params := client.Params{}
	params.AddParam("chat_id", event.ChatID)

	switch {
	case strings.HasSuffix(text, cancelCmdA) || strings.HasSuffix(text, cancelCmdB):
		meta.Prefix = ""
		meta.IsPrefixSet = false
		params.AddParam("reply_markup", mainKeyboard)
		if show, ok := meta.ActiveShows[event.ChatID]; ok {
			msg := fmt.Sprintf(msgCancel, show.Name)
			params.AddParam("text", msg)
		} else {
			params.AddParam("text", msgCancelNew)
		}

		return p.tg.SendMessage(params)

	case strings.HasPrefix(text, addCmdA) || strings.HasPrefix(text, addCmdB):

		if !meta.IsPrefixSet {
			params.AddParam("reply_markup", cancelKeyboard)
			params.AddParam("text", msgAddInfo)
			meta.Prefix = "/add"
			meta.IsPrefixSet = true
			return p.tg.SendMessage(params)
		}

		return p.addNewTvShow(event, meta)

	case strings.HasPrefix(text, updCmdA) || strings.HasPrefix(text, updCmdB):

		if _, ok := meta.ActiveShows[event.ChatID]; !ok {
			params.AddParam("text", msgNoAddedShows)
			return p.tg.SendMessage(params)
		}

		if !meta.IsPrefixSet {
			params.AddParam("reply_markup", cancelKeyboard)
			msg := fmt.Sprintf(msgUpdateInfo, meta.ActiveShows[event.ChatID].Name)
			params.AddParam("text", msg)
			meta.Prefix = "/upd"
			meta.IsPrefixSet = true
			return p.tg.SendMessage(params)
		}

		return p.updateTvShow(event, meta)

	case strings.HasSuffix(text, listCmdA) || strings.HasSuffix(text, listCmdB):
		// TODO: list command

	case text == startCmd:
		meta.ActiveShows[event.ChatID] = events.ActiveShow{}
		return p.addNewUser(event)
	case text == helpCmd:
		params.AddParam("text", msgHelp)
		return p.tg.SendMessage(params)
	default:
		params.AddParam("text", msgUnknownCommand)
		return p.tg.SendMessage(params)
	}

	return nil
}

func (p *Processor) updateTvShow(event *events.Event, meta *events.Meta) error {
	defer func() {
		meta.Prefix = ""
		meta.IsPrefixSet = false
	}()

	params := client.Params{}
	params.AddParam("chat_id", event.ChatID)
	params.AddParam("reply_markup", mainKeyboard)

	episode, err := strconv.Atoi(strings.SplitN(event.Text, " ", 2)[1])
	if err != nil {
		params.AddParam("text", msgInvalidEpisode)
		return p.tg.SendMessage(params)
	}

	updatedTvShow := &storage.TvShow{
		Name:            meta.ActiveShows[event.ChatID].Name,
		Season:          meta.ActiveShows[event.ChatID].Season,
		Episode:         episode,
		UsersTelegramID: event.ChatID,
	}

	if err := p.storage.UpdateLastWatchedEpisode(updatedTvShow); err != nil {
		return e.Wrap("can't update last watched episode", err)
	}

	meta.ActiveShows[event.ChatID] = events.ActiveShow{
		Name:    meta.ActiveShows[event.ChatID].Name,
		Season:  meta.ActiveShows[event.ChatID].Season,
		Episode: episode,
	}

	msg := fmt.Sprintf(msgUpdated, meta.ActiveShows[event.ChatID].Name)
	params.AddParam("text", msg)
	if err := p.tg.SendMessage(params); err != nil {
		return e.Wrap("can't update last watched episode", err)
	}

	return nil
}

func (p *Processor) addNewTvShow(event *events.Event, meta *events.Meta) error {
	defer func() {
		meta.Prefix = ""
		meta.IsPrefixSet = false
	}()

	errMsg := "can't add new Tv Show"

	params := client.Params{}
	params.AddParam("chat_id", event.ChatID)
	params.AddParam("reply_markup", mainKeyboard)

	// Get inputs after second split
	inputs := strings.Split(strings.SplitN(event.Text, " ", 2)[1], "/")
	if len(inputs) != 3 {
		params.AddParam("text", msgInvalidInput)
		return p.tg.SendMessage(params)
	}

	season, err := strconv.Atoi(inputs[1])
	if err != nil {
		params.AddParam("text", msgInvalidInput)
		return p.tg.SendMessage(params)
	}

	episode, err := strconv.Atoi(inputs[2])
	if err != nil {
		params.AddParam("text", msgInvalidInput)
		return p.tg.SendMessage(params)
	}

	show := &storage.TvShow{
		Name:            inputs[0],
		Season:          season,
		Episode:         episode,
		UsersTelegramID: event.ChatID,
	}

	exists, err := p.storage.IsTvShowExists(show)
	if err != nil {
		return e.Wrap(errMsg, err)
	}

	if exists {
		msg := fmt.Sprintf(msgAlreadyExists, meta.ActiveShows[event.ChatID].Name)
		params.AddParam("text", msg)
		return p.tg.SendMessage(params)
	}

	if err := p.storage.SaveTvShow(show); err != nil {
		return e.Wrap(errMsg, err)
	}

	meta.ActiveShows[event.ChatID] = events.ActiveShow{
		Name:    inputs[0],
		Season:  season,
		Episode: episode,
	}

	msg := fmt.Sprintf(msgAdded, meta.ActiveShows[event.ChatID].Name)
	params.AddParam("text", msg)
	if err := p.tg.SendMessage(params); err != nil {
		return e.Wrap(errMsg, err)
	}

	return nil
}

func (p *Processor) addNewUser(event *events.Event) error {
	user := &storage.User{
		TelegramID: event.ChatID,
		Username:   event.Username,
	}

	if err := p.storage.CreateUser(user); err != nil {
		return e.Wrap("can't add new user", err)
	}

	params := client.Params{}
	params.AddParam("chat_id", event.ChatID)
	params.AddParam("text", fmt.Sprintf(msgHello, event.FirstName))
	params.AddParam("parse_mode", "Markdown")

	if err := p.tg.SendMessage(params); err != nil {
		e.Wrap("can't add new user", err)
	}

	return nil
}
