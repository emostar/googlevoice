// mautrix-gvoice - A Matrix-GVoice puppeting bridge.
// Copyright (C) 2022 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package database

import (
	"database/sql"
	"sync"
	"time"

	log "maunium.net/go/maulogger/v2"

	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/util/dbutil"

	"go.mau.fi/whatsmeow/types"
)

type UserQuery struct {
	db  *Database
	log log.Logger
}

func (uq *UserQuery) New() *User {
	return &User{
		db:  uq.db,
		log: uq.log,

		lastReadCache: make(map[PortalKey]time.Time),
		inSpaceCache:  make(map[PortalKey]bool),
	}
}

func (uq *UserQuery) GetAll() (users []*User) {
	rows, err := uq.db.Query(`SELECT mxid, primary_did, management_room, space_room, timezone FROM "user"`)
	if err != nil || rows == nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		users = append(users, uq.New().Scan(rows))
	}
	return
}

func (uq *UserQuery) GetByMXID(userID id.UserID) *User {
	row := uq.db.QueryRow(
		`SELECT mxid, primary_did, management_room, space_room, timezone FROM "user" WHERE mxid=$1`,
		userID,
	)
	if row == nil {
		return nil
	}
	return uq.New().Scan(row)
}

func (uq *UserQuery) GetByUsername(username string) *User {
	row := uq.db.QueryRow(
		`SELECT mxid, primary_did, management_room, space_room, timezone FROM "user" WHERE username=$1`,
		username,
	)
	if row == nil {
		return nil
	}
	return uq.New().Scan(row)
}

type User struct {
	db  *Database
	log log.Logger

	MXID            id.UserID
	JID             types.JID
	ManagementRoom  id.RoomID
	SpaceRoom       id.RoomID
	PrimaryDID      string
	PhoneLastSeen   time.Time // TODO Remove
	PhoneLastPinged time.Time // TODO Remove
	Timezone        string

	lastReadCache     map[PortalKey]time.Time
	lastReadCacheLock sync.Mutex
	inSpaceCache      map[PortalKey]bool
	inSpaceCacheLock  sync.Mutex
}

func (user *User) Scan(row dbutil.Scannable) *User {
	var timezone, primaryDID sql.NullString
	err := row.Scan(
		&user.MXID, &primaryDID, &user.ManagementRoom, &user.SpaceRoom,
		&timezone,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			user.log.Errorln("Database scan failed:", err)
		}
		return nil
	}
	user.Timezone = timezone.String
	if len(primaryDID.String) > 0 {
		user.JID = types.NewADJID(primaryDID.String, 1, 1) // TODO Change JID to just an ID
	}
	return user
}

func (user *User) usernamePtr() *string {
	if !user.JID.IsEmpty() {
		return &user.JID.User
	}
	return nil
}

func (user *User) agentPtr() *uint8 {
	if !user.JID.IsEmpty() {
		return &user.JID.Agent
	}
	return nil
}

func (user *User) devicePtr() *uint8 {
	if !user.JID.IsEmpty() {
		return &user.JID.Device
	}
	return nil
}

func (user *User) phoneLastSeenPtr() *int64 {
	if user.PhoneLastSeen.IsZero() {
		return nil
	}
	ts := user.PhoneLastSeen.Unix()
	return &ts
}

func (user *User) phoneLastPingedPtr() *int64 {
	if user.PhoneLastPinged.IsZero() {
		return nil
	}
	ts := user.PhoneLastPinged.Unix()
	return &ts
}

func (user *User) Insert() {
	_, err := user.db.Exec(
		`INSERT INTO "user" (mxid, primary_did, management_room, space_room, timezone) VALUES ($1, $2, $3, $4, $5)`,
		user.MXID, user.PrimaryDID, user.ManagementRoom, user.SpaceRoom,
		user.Timezone,
	)
	if err != nil {
		user.log.Warnfln("Failed to insert %s: %v", user.MXID, err)
	}
}

func (user *User) Update() {
	_, err := user.db.Exec(
		`UPDATE "user" SET primary_did=$1, management_room=$2, space_room=$3, timezone=$4 WHERE mxid=$5`,
		user.PrimaryDID, user.ManagementRoom, user.SpaceRoom, user.Timezone,
		user.MXID,
	)
	if err != nil {
		user.log.Warnfln("Failed to update %s: %v", user.MXID, err)
	}
}

func (user *User) GetLastAppStateKeyID() ([]byte, error) {
	var keyID []byte
	err := user.db.QueryRow("SELECT key_id FROM whatsmeow_app_state_sync_keys ORDER BY timestamp DESC LIMIT 1").Scan(&keyID)
	return keyID, err
}
