/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ernestaws

// Event : Generic event interface
type Event interface {
	Validate() error
	Process() (err error)
	Error(err error)
	Create() error
	Update() error
	Delete() error
	Get() error
	GetSubject() string
	GetBody() []byte
}
