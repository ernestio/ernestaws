/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package template

import (
	"encoding/json"
	"errors"
	"log"

	"github.com/ernestio/ernestaws"
)

// Event stores the template data
type Event struct {
	ProviderType  string `json:"_provider"`
	ComponentType string `json:"_component"`
	ComponentID   string `json:"_component_id"`
	State         string `json:"_state"`
	Action        string `json:"_action"`
	ErrorMessage  string `json:"error,omitempty"`
	Subject       string `json:"-"`
	Body          []byte `json:"-"`
	CryptoKey     string `json:"-"`
}

// New : Constructor
func New(subject string, body []byte, cryptoKey string) ernestaws.Event {
	n := Event{Subject: subject, Body: body, CryptoKey: cryptoKey}

	return &n
}

// GetBody : Gets the body for this event
func (ev *Event) GetBody() []byte {
	var err error
	if ev.Body, err = json.Marshal(ev); err != nil {
		log.Println(err.Error())
	}
	return ev.Body
}

// GetSubject : Gets the subject for this event
func (ev *Event) GetSubject() string {
	return ev.Subject
}

// Process : starts processing the current message
func (ev *Event) Process() (err error) {
	if err := json.Unmarshal(ev.Body, &ev); err != nil {
		ev.Error(err)
		return err
	}

	if err := ev.Validate(); err != nil {
		ev.Error(err)
		return err
	}

	return nil
}

// Error : Will respond the current event with an error
func (ev *Event) Error(err error) {
	log.Printf("Error: %s", err.Error())
	ev.ErrorMessage = err.Error()
	ev.State = "errored"

	ev.Body, err = json.Marshal(ev)
}

// Complete : sets the state of the event to completed
func (ev *Event) Complete() {
	ev.State = "completed"
}

// Validate checks if all criteria are met
func (ev *Event) Validate() error {
	return nil
}

// Find : Find an object on aws
func (ev *Event) Find() error {
	return errors.New(ev.Subject + " not supported")
}

// Create : Creates a nat object on aws
func (ev *Event) Create() error {
	return errors.New(ev.Subject + " not supported")
}

// Update : Updates a nat object on aws
func (ev *Event) Update() error {
	return errors.New(ev.Subject + " not supported")
}

// Delete : Deletes a nat object on aws
func (ev *Event) Delete() error {
	return errors.New(ev.Subject + " not supported")
}

// Get : Gets a nat object on aws
func (ev *Event) Get() error {
	return errors.New(ev.Subject + " not supported")
}
