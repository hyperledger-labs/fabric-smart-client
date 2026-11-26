/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package chat is a chat on FSC without a ledger: a server node keeps channels and fans
// channel messages out to their members, and clients send each other direct messages.
package chat

// Op is an operation a client asks the server for.
type Op string

// The operations of Request.
const (
	OpLogin  Op = "login"
	OpLogout Op = "logout"
	OpWho    Op = "who"
	OpJoin   Op = "join"
	OpLeave  Op = "leave"
	OpPost   Op = "post"
)

// Request is what a client sends the server. The server takes the sender from the session,
// so a request carries no name.
type Request struct {
	Op      Op     `json:"op"`
	Channel string `json:"channel,omitempty"`
	Text    string `json:"text,omitempty"`
}

// User is a logged-in user and the organization of its node.
type User struct {
	Name string `json:"name"`
	Org  string `json:"org"`
}

// Reply is the answer to a Request, and the acknowledgement of a delivered Message.
type Reply struct {
	// Users is set by login and who.
	Users []User `json:"users,omitempty"`
	// Failed lists the members a post did not reach.
	Failed []string `json:"failed,omitempty"`
	// Error is set when the request was refused.
	Error string `json:"error,omitempty"`
}

// Message is a chat message as a client receives it.
type Message struct {
	// Channel is empty for a direct message.
	Channel string `json:"channel,omitempty"`
	From    string `json:"from"`
	Text    string `json:"text"`
}
