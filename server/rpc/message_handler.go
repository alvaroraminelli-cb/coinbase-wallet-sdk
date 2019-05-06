// Copyright (c) 2019 Coinbase, Inc. See LICENSE

package rpc

import (
	"fmt"

	"github.com/CoinbaseWallet/walletlinkd/store"
	"github.com/CoinbaseWallet/walletlinkd/store/models"
	"github.com/pkg/errors"
)

const (
	// HostMessageHostSession - host session
	HostMessageHostSession = "hostSession"

	// GuestMessageJoinSession - join session
	GuestMessageJoinSession = "joinSession"
)

// MessageHandler - handles rpc messages
type MessageHandler struct {
	session *models.Session

	sendCh chan<- interface{}
	subCh  chan interface{}

	store  store.Store
	pubSub *PubSub
}

// NewMessageHandler - construct a MessageHandler
func NewMessageHandler(
	sendCh chan<- interface{},
	sto store.Store,
	pubSub *PubSub,
) (*MessageHandler, error) {
	if sendCh == nil {
		return nil, errors.Errorf("sendCh must not be nil")
	}
	if sto == nil {
		return nil, errors.Errorf("store must not be nil")
	}
	if pubSub == nil {
		return nil, errors.Errorf("pubSub must not be nil")
	}
	return &MessageHandler{
		sendCh: sendCh,
		subCh:  make(chan interface{}),
		store:  sto,
		pubSub: pubSub,
	}, nil
}

// Handle - handle an RPC message
func (c *MessageHandler) Handle(req *Request) (ok bool) {
	var res *Response

	if req.ID < 1 {
		res = errorResponse(req.ID, "invalid request ID", true)
	} else {
		switch req.Message {
		case HostMessageHostSession:
			res = c.handleHostSession(req.ID, req.Data)

		case GuestMessageJoinSession:
			res = c.handleJoinSession(req.ID, req.Data)
		}
	}

	if res == nil {
		res = errorResponse(req.ID, "unsupported message", true)
	}

	c.sendCh <- res

	return !res.Fatal
}

// Close - clean up
func (c *MessageHandler) Close() {
	c.pubSub.UnsubscribeAll(c.subCh)
	close(c.subCh)
}

func (c *MessageHandler) handleHostSession(
	requestID int,
	data map[string]string,
) *Response {
	sessionID, sessionKey := data["id"], data["key"]

	res, session := c.findSession(requestID, sessionID, sessionKey)
	if res != nil {
		return res
	}

	if session == nil {
		// there isn't an existing session; persist the new session
		session = &models.Session{ID: sessionID, Key: sessionKey}
		if err := c.store.Set(session.StoreKey(), session); err != nil {
			fmt.Println(errors.Wrap(err, "failed to persist session"))
			return errorResponse(requestID, "internal error", true)
		}
	}

	c.session = session
	c.pubSub.Subscribe(hostPubSubID(sessionID), c.subCh)

	return &Response{RequestID: requestID}
}

func (c *MessageHandler) handleJoinSession(
	requestID int,
	data map[string]string,
) *Response {
	sessionID, sessionKey := data["id"], data["key"]

	res, session := c.findSession(requestID, sessionID, sessionKey)
	if res != nil {
		return res
	}

	if session == nil {
		// there isn't an existing session; fail
		errMsg := fmt.Sprintf("no such session: %s", sessionID)
		return errorResponse(requestID, errMsg, false)
	}

	c.session = session
	c.pubSub.Subscribe(guestPubSubID(sessionID), c.subCh)

	return &Response{RequestID: requestID}
}

func (c *MessageHandler) findSession(
	requestID int,
	sessionID string,
	sessionKey string,
) (*Response, *models.Session) {
	if !models.IsValidSessionID(sessionID) {
		return errorResponse(requestID, "invalid session id", true), nil
	}
	if !models.IsValidSessionKey(sessionKey) {
		return errorResponse(requestID, "invalid session key", true), nil
	}

	session := &models.Session{ID: sessionID}
	ok, err := c.store.Get(session.StoreKey(), session)
	if err != nil {
		fmt.Println(errors.Wrap(err, "failed to load session"))
		return errorResponse(requestID, "internal error", true), nil
	}

	if !ok {
		return nil, nil
	}

	// there is an existing session; check that session key matches
	if session.Key != sessionKey {
		return errorResponse(requestID, "incorrect session key", true), nil
	}

	return nil, session
}

func errorResponse(requestID int, errorMessage string, fatal bool) *Response {
	return &Response{
		RequestID: requestID,
		Error:     errorMessage,
		Fatal:     fatal,
	}
}

func hostPubSubID(sessionID string) string {
	return "h." + sessionID
}

func guestPubSubID(sessionID string) string {
	return "g." + sessionID
}
