/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package ernestaws

import (
	"strings"
)

// Handle : Handles the given event
func Handle(ev *Event) (string, []byte) {
	var err error

	n := *ev
	if err = n.Process(); err != nil {
		return n.GetSubject() + ".error", n.GetBody()
	}

	parts := strings.Split(n.GetSubject(), ".")
	switch parts[1] {
	case "create":
		err = n.Create()
	case "update":
		err = n.Update()
	case "delete":
		err = n.Delete()
	case "get":
		err = n.Get()
	case "find":
		err = n.Find()
	}

	if err != nil {
		n.Error(err)
		return n.GetSubject() + ".error", n.GetBody()
	}

	n.Complete()

	return n.GetSubject() + ".done", n.GetBody()
}
