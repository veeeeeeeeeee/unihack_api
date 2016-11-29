package main

import (
	"database/sql"
	"strings"
)

func getUserDeviceID(userID string) (string, error) {

	var device string
	err := db.QueryRow("select contact_details->'device_id' from users WHERE user_id=$1 LIMIT 1", userID).Scan(&device)

	if err != nil {
		return device, err
	}

	device = strings.Replace(device, "\"", "", -1)

	return device, nil
}

func getSimpleContacts(rows *sql.Rows) ([]SimpleContact, error) {
	var nearby []SimpleContact

	defer rows.Close()
	for rows.Next() {
		var n SimpleContact
		var image sql.NullString
		var first sql.NullString
		var userID string
		if err := rows.Scan(&image, &first, &userID); err != nil {
			return nil, err
		}

		n.FirstName = first.String
		n.Image = image.String
		n.UserID = userID

		nearby = append(nearby, n)
	}

	return nearby, nil
}
