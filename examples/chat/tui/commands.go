/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package tui is the terminal user interface of the chat client.
package tui

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Kind is what a line typed by the user asks for.
type Kind int

// The kinds of Command.
const (
	KindSay Kind = iota
	KindJoin
	KindLeave
	KindMsg
	KindWho
	KindQuit
)

// Command is a parsed input line.
type Command struct {
	Kind Kind
	// Arg is the channel of join and leave, or the user of msg.
	Arg string
	// Text is the message of say and msg.
	Text string
}

const usage = "commands: /join <channel>, /leave [channel], /msg <user> <text>, /who, /quit"

// Parse parses a line: a slash command, or text to post to the current channel.
func Parse(line string) (Command, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Command{}, errors.New("nothing to send")
	}
	if !strings.HasPrefix(line, "/") {
		return Command{Kind: KindSay, Text: line}, nil
	}
	fields := strings.Fields(line)
	args := fields[1:]
	switch fields[0] {
	case "/join":
		if len(args) != 1 {
			return Command{}, errors.New("usage: /join <channel>")
		}
		return Command{Kind: KindJoin, Arg: strings.ToLower(args[0])}, nil
	case "/leave":
		if len(args) > 1 {
			return Command{}, errors.New("usage: /leave [channel]")
		}
		return Command{Kind: KindLeave, Arg: strings.ToLower(strings.Join(args, ""))}, nil
	case "/msg":
		if len(args) < 2 {
			return Command{}, errors.New("usage: /msg <user> <text>")
		}
		_, text, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "/msg")), args[0])
		return Command{Kind: KindMsg, Arg: args[0], Text: strings.TrimSpace(text)}, nil
	case "/who":
		return Command{Kind: KindWho}, nil
	case "/quit":
		return Command{Kind: KindQuit}, nil
	default:
		return Command{}, errors.New(usage)
	}
}
