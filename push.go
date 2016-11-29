package main

import (
	"fmt"
	"log"

	"github.com/anachronistic/apns"
)

func pushMessage(message string, token string) {
	go func() {
		payload := apns.NewPayload()
		payload.Alert = message

		pn := apns.NewPushNotification()
		pn.DeviceToken = token
		pn.AddPayload(payload)

		client := apns.NewClient("gateway.sandbox.push.apple.com:2195", "cert.pem", "key.pem")
		resp := client.Send(pn)

		alert, err := pn.PayloadString()

		if err != nil {
			log.Printf("Error pn.PayloadString: %s", err)
		}

		fmt.Println("  Alert:", alert)
		fmt.Println("Success:", resp.Success)
		fmt.Println("  Error:", resp.Error)
	}()
}
