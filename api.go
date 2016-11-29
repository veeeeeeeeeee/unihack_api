package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pborman/uuid"
)

var version = "0.1"
var db *sqlx.DB

func versionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", version)
}

func signupHandler(w http.ResponseWriter, r *http.Request) {

	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	jsData := make(map[string]interface{})

	err = json.Unmarshal(body, &jsData)

	fmt.Printf("data: %+v\n", jsData)

	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID := jsData["user_id"]

	_, err = db.Exec("INSERT INTO users (user_id, contact_details) VALUES ($1, $2)", userID, body)

	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Printf("Body: %s\n", body)

	w.WriteHeader(http.StatusCreated)
}

// Location stores a location
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	UserID    string  `json:"user_id"`
}

func locationCreateHandler(w http.ResponseWriter, r *http.Request) {

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var location Location

	err = json.Unmarshal(body, &location)

	fmt.Printf("Location: %+v\n", location)

	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = db.Exec("INSERT INTO locations (latitude, longitude, created, user_id_users) VALUES ($1, $2, $3, $4)", location.Latitude, location.Longitude, time.Now(), location.UserID)

	if err != nil {
		log.Printf("err: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fmt.Printf("Body: %s\n", body)

	w.WriteHeader(http.StatusCreated)
}

// SimpleContact blah
type SimpleContact struct {
	Image     string `json:"image"`
	FirstName string `json:"first_name"`
	UserID    string `json:"user_id"`
}

func nearbyHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]

	log.Printf("Fetching nearby for User ID: %s", userID)

	rows, err := db.Query("select contact_details->>'image' AS image, contact_details->>'first_name' as first_name, contact_details->>'user_id' as user_id from users WHERE user_id != $1 AND user_id NOT IN (SELECT \"to\" FROM requests WHERE \"from\" = $1 AND allowed=true)", userID)

	if err != nil {
		log.Printf("err 1: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nearby, err := getSimpleContacts(rows)

	if err != nil {
		log.Printf("err 2: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Nearby: %d contacts", len(nearby))

	nearbyJSON, err := json.Marshal(nearby)

	if err != nil {
		log.Printf("err 2: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("len(json): %d", len(nearbyJSON))

	// log.Printf("Nearby: %+v", nearbyJSON)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "%s", nearbyJSON)
}

func createRequestHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	from := vars["from"]
	to := vars["to"]

	_, err := db.Exec("INSERT INTO requests (id, \"from\", \"to\", created) VALUES ($1, $2, $3, $4)", uuid.NewUUID(), from, to, time.Now())

	if err != nil {
		log.Printf("err 1: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Fetch the device ID of the from person:
	device, err := getUserDeviceID(to)

	if err != nil {
		// Not so serious if we can't get device ID
		log.Printf("err 2: %s", err)
	} else {
		pushMessage("Someone has requested your contact details!", device)
	}

	w.WriteHeader(http.StatusCreated)
}

func confirmRequestHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	requestID := vars["request_id"]
	answer := vars["answer"]

	allowed := false

	if answer == "true" {
		allowed = true
	}

	var userID string
	err := db.QueryRow("UPDATE requests SET allowed=$1 WHERE id=$2 RETURNING \"from\"", allowed, requestID).Scan(&userID)

	if err != nil {
		log.Printf("err 1: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if allowed {
		// Create a request for going the other way as well, so that details are available to both:

		// Let user know
		device, err := getUserDeviceID(userID)

		if err != nil {
			log.Printf("err 1: %s", err)
		} else {
			pushMessage("Your request has been accepted!", device)
		}
	}

	w.WriteHeader(http.StatusCreated)
}

// Request request
type Request struct {
	SimpleContact
	RequestID string `json:"request_id"`
}

func pendingRequestsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	userID := vars["user_id"]

	rows, err := db.Query("select contact_details->>'image' AS image, contact_details->>'first_name' as first_name, contact_details->>'user_id' as user_id, requests.id as request_id from users INNER JOIN requests ON requests.\"from\" = users.user_id WHERE requests.\"to\" = $1 AND allowed IS NULL", userID)

	if err != nil {
		log.Printf("err 1: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var requests []Request

	defer rows.Close()
	for rows.Next() {
		var n Request
		var image sql.NullString
		var first sql.NullString
		if err = rows.Scan(&image, &first, &n.UserID, &n.RequestID); err != nil {
			if err != nil {
				log.Printf("err scanning: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		// n.FirstName = strings.TrimRight(strings.TrimLeft(n.FirstName, "\""), "\"")
		// n.Image = strings.TrimRight(strings.TrimLeft(n.Image, "\""), "\"")
		// n.UserID = strings.TrimRight(strings.TrimLeft(n.UserID, "\""), "\"")

		n.FirstName = first.String
		n.Image = image.String

		requests = append(requests, n)
	}

	nearbyJSON, err := json.Marshal(requests)

	if err != nil {
		log.Printf("err 2: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Nearby: %+v", nearbyJSON)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "%s", nearbyJSON)
}

func grantedRequestsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	userID := vars["user_id"]

	_ = userID

	rows, err := db.Query("SELECT contact_details FROM users WHERE user_id IN (SELECT \"to\" FROM requests WHERE allowed=true AND \"from\"=$1) ORDER BY contact_details->>'last_name', contact_details->>'first_name'", userID)

	if err != nil {
		log.Printf("err 1: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var details []map[string]interface{}

	defer rows.Close()
	for rows.Next() {
		var detail string
		if err = rows.Scan(&detail); err != nil {
			if err != nil {
				log.Printf("err scanning: %s", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		var detailMap map[string]interface{}
		err = json.Unmarshal([]byte(detail), &detailMap)

		if err != nil {
			log.Printf("err scanning: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		delete(detailMap, "device_id")

		details = append(details, detailMap)
	}

	nearbyJSON, err := json.Marshal(details)

	if err != nil {
		log.Printf("err 2: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Printf("Nearby: %+v", nearbyJSON)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", nearbyJSON)
}

func main() {
	var err error
	r := mux.NewRouter()

	sqlUser := os.Getenv("SQL_USER")
	sqlDb := os.Getenv("SQL_DB")
	sqlHost := os.Getenv("SQL_HOST")
	sqlPass := os.Getenv("SQL_PASS")

	sqlString := fmt.Sprintf("user=%s dbname=%s password=%s host=%s sslmode=disable", sqlUser, sqlDb, sqlPass, sqlHost)

	db, err = sqlx.Open("postgres", sqlString)

	if err != nil {
		panic(err)
	}

	err = db.Ping()

	if err != nil {
		panic(err)
	}

	r.HandleFunc("/v1/version", versionHandler)
	r.HandleFunc("/v1/user/create", signupHandler)
	r.HandleFunc("/v1/location/create", locationCreateHandler)
	r.HandleFunc("/v1/nearby/{user_id}", nearbyHandler)
	r.HandleFunc("/v1/requests/create/{from}/{to}", createRequestHandler)
	r.HandleFunc("/v1/requests/allowed/{request_id}/{answer}", confirmRequestHandler)
	r.HandleFunc("/v1/requests/list/{user_id}", pendingRequestsHandler)
	r.HandleFunc("/v1/requests/granted/{user_id}", grantedRequestsHandler) // List of contact details you've been granted

	http.Handle("/", r)

	port := os.Getenv("CH_PORT")

	err = http.ListenAndServe(":"+port, nil)

	if err != nil {
		fmt.Printf("error: %s\n", err)
	}
}
