// Copyright 2017 - webofmars - Frederic Leger <contact@webofmars.com>

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/jasonlvhit/gocron"
	"github.com/pborman/getopt/v2"
	"github.com/tkanos/gonfig"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// globals vars

// Configuration : gonfig struct
type Configuration struct {
	CalendarID             string
	GoogleAPISecretFile    string
	UserID                 string
	TimezoneName           string
	CheckAtStartup         bool
	CheckFirstAt           string
	CheckInterval          string
	BuddiesList            map[string]string
	SlackWebhookURL        string
	SlackChannel           string
	NotificationInterval   uint64
	_CheckIntervalDuration time.Duration
	_Timezone              *time.Location
}

// OncallBuddy : oncall buddy struct
type OncallBuddy struct {
	Name        string
	PhoneNumber string
}

// OncallBuddies : Map of Usernames and Phone Numbers of Buddies
var OncallBuddies map[string]string

// config : Configuration instance with defaults values
var config = Configuration{
	GoogleAPISecretFile:    "/etc/oncall-buddy-finder/client_secret.json",
	UserID:                 "me@example.com",
	CalendarID:             "",
	TimezoneName:           "UTC",
	CheckAtStartup:         true,
	CheckFirstAt:           "08:00",
	CheckInterval:          "60s",
	BuddiesList:            OncallBuddies,
	SlackWebhookURL:        "",
	SlackChannel:           "#general",
	NotificationInterval:   12,
	_CheckIntervalDuration: 128,
	_Timezone:              time.UTC,
}

var ctx = context.Background()
var googleAPIClient *http.Client
var currentBuddy OncallBuddy

// TODOs:
// 		TODO: should not fail if no config file and should rely on env & defaults
// 		TODO: How to get the phone numbers through the env variables ?
// 		TODO: implement a sanity check

/********************************************************************
 * Functions
 ********************************************************************/

// set some vars & load config file based on env values
func setup() {
	spew.Config.Indent = "\t"

	// get the correct config file
	var configFilePath string
	configFilePath = GuessConfigurationFilename()
	spew.Printf("INFO: Using %s config file\n", configFilePath)

	// load the configuration from file
	LoadConfiguration(configFilePath, &config)
	config._Timezone = getTimezone(config.TimezoneName)
	config._CheckIntervalDuration = getCheckIntervalDuration(config.CheckInterval)
}

// GuessConfigurationFilename :
/* 	try to guess the config file to use (based on $CONFIG or $ENV)
 *	if it can't will return a default one */
func GuessConfigurationFilename() (filename string) {
	var forced string = os.Getenv("CONFIG")
	var env string = os.Getenv("ENV")

	// given on command line
	getopt.FlagLong(&filename, "config", 0, "the config file path")
	getopt.Parse()
	if len(filename) > 0 {
		_, err := os.Stat(filename)
		if err == nil {
			return filename
		}
		log.Fatalf("ERROR: can't load %s config file, skipping\n", filename)
	}

	// passed through $CONFIG
	if len(forced) > 0 {
		_, err := os.Stat(forced)
		if err == nil {
			return forced
		}
		// this is fatal because if this is forced we must trust it, no ?
		log.Fatalf("ERROR: can't load %s config file, skipping\n", forced)
	}

	// CWD through $ENV
	if len(env) > 0 {
		path, err := filepath.Abs("oncall-buddy-finder." + env + ".json")
		_, err = os.Stat(path)
		if err == nil {
			return path
		}
		spew.Printf("WARN: can't load %s config file, skipping\n", path)
	}

	return "oncall-buddy-finder.json"
}

/* LoadConfiguration :
 * 	Load the config from guessed config file paths and keeps defaults
 *
 * 	The precedence order is:
 * 		- defaults values
 * 		- config file values
 * 		- env values
 */
func LoadConfiguration(configFilePath string, config *Configuration) {

	err := gonfig.GetConf(configFilePath, config)
	if err != nil {
		log.Fatalf("Can't load configuration file: %s\n", err)
	}
}

/* setupGoogleAPIAccess:
 *   Setup a correct connexion to Google API services
 */
func setupGoogleAPIAccess() {

	b, err := ioutil.ReadFile(config.GoogleAPISecretFile)
	if err != nil {
		log.Fatalf("Unable to read googleAPIClient secret file: %v", err)
	}

	googleAPIConfig, err := google.JWTConfigFromJSON(b, calendar.CalendarEventsReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	googleAPIConfig.Subject = config.UserID

	googleAPIClient = googleAPIConfig.Client(ctx)
}

/* getTimezone:
 *   load correct Timezone based on the name provided
 */
func getTimezone(timezoneStr string) (timezone *time.Location) {

	timezone, err := time.LoadLocation(timezoneStr)

	if err != nil {
		log.Fatalf("Unable to load timezone")
	}
	return
}

/* getCheckIntervalDuration:
 *   Parse the configuration check interval (a string) into time.Duration
 */
func getCheckIntervalDuration(interval string) time.Duration {
	d1, err := time.ParseDuration(config.CheckInterval)
	if err != nil {
		spew.Errorf("Can't parse time interval %s", interval)
	}
	return d1
}

/* getCalendarEvents:
 *   Get all the Google Calendar events
 */
func getCalendarEvents(CalendarID string, TimeMin time.Time, TimeMax time.Time, TimeZone *time.Location, MaxResults int64) (*calendar.Events, error) {

	svc, err := calendar.New(googleAPIClient)
	if err != nil {
		spew.Printf("Error: Unable to create Calendar service: %v", err)
		return nil, err
	}

	spew.Printf("TimeMin : %s\n", TimeMin.Format(time.RFC3339))
	spew.Printf("TimeMax : %s\n", TimeMax.Format(time.RFC3339))

	// get the calendars events
	events, err := svc.Events.List(config.CalendarID).
		TimeZone(TimeZone.String()).
		TimeMin(TimeMin.Format(time.RFC3339)).
		TimeMax(TimeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()

	if err != nil {
		spew.Printf("Error: Unable to get calendar events: %v", err)
		return nil, err
	}

	return events, err
}

/* printEventsInfos:
 *   Prints events (used for debug mostly)
 */
func printEventsInfos(events *calendar.Events) {
	if len(events.Items) > 0 {
		for _, i := range events.Items {
			spew.Printf("entry: %s - start: %v\n", i.Summary, i.Start.DateTime)
		}
	}
}

/* getCurrentBuddy:
 *   Find the name of the current buddy who is oncall
 */
func getCurrentBuddy(interval time.Duration) (buddy OncallBuddy, err error) {
	spew.Printf("interval : %s\n", interval)

	// get the timeframe
	tmin := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), time.Now().Hour(), time.Now().Minute(), int(00), int(0), time.Now().Location())
	tmax := tmin.Add(interval)

	// get the calendar events for this timeframe
	events, err := getCalendarEvents(config.CalendarID, tmin, tmax, config._Timezone, 1)
	if err != nil {
		spew.Printf("Error: Unable to fetch events from calendar : %v", err)
		return
	}

	// print the result
	printEventsInfos(events)

	var lastOptionEvent *calendar.Event

	if len(events.Items) > 0 {
		for _, i := range events.Items {
			if i.Start.DateTime == "" {
				// this is a full day event, keep it as last option
				lastOptionEvent = i
				continue
			} else if i.Start.Date == "" {
				// this is a normal event (and should be prefered)
				if i.Summary != "" {
					buddy.Name = strings.ToLower(i.Summary)
					buddy.PhoneNumber, err = getCurrentBuddyPhone(buddy.Name)
				}
				return
			}
		}
		// if we are still here, there was no well formated normal event
		buddy.Name = strings.ToLower(lastOptionEvent.Summary)
		buddy.PhoneNumber, err = getCurrentBuddyPhone(buddy.Name)
	}

	return
}

/* getCurrentBuddyPhone:
 *   Find the phone number of the current buddy who is oncall
 */
func getCurrentBuddyPhone(name string) (phoneNumber string, err error) {
	phoneNumber, exists := config.BuddiesList[name]
	if !exists {
		err = fmt.Errorf("ERROR: %s phone number is not known in the buddies list: %s", name, config.BuddiesList)
		return
	}
	return
}

/* serveCurrentBuddy:
 *   Handler to server current buddy through a REST API
 */
func serveCurrentBuddy(w http.ResponseWriter, r *http.Request) {
	spew.Printf("Served http request : %s %s %s %s %s\n", r.RemoteAddr, r.Method, r.Host, r.RequestURI, r.UserAgent())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentBuddy)
}

/* SlackNotification:
 *   Notify the current buddy by slack
 */
func SlackNotification(msg string, SlackWebhookURL string, slackChannelName string) []error {

	payload := slack.Payload{
		Text:      msg,
		Channel:   slackChannelName,
		LinkNames: "1",
	}

	err := slack.Send(SlackWebhookURL, "", payload)
	if len(err) > 0 {
		spew.Printf("ERROR: Unable to send slack notification: %s\n", err)
		return err
	}
	return nil
}

/* BuddyWatcherTask:
 *   The main task that will be executed in loop
 */
func BuddyWatcherTask() {

	spew.Print("running buddy watcher task\n")

	var err error
	var previousBuddy OncallBuddy
	previousBuddy = currentBuddy

	buddy, err := getCurrentBuddy(config._CheckIntervalDuration)
	if err != nil {
		spew.Errorf("Can't retrieve current buddy")
	}

	// FIXME: detection of the empty buddy could be smarter
	if buddy == (OncallBuddy{}) {
		spew.Errorf("Buddy was empty, should it be ?")
	}

	if buddy.Name > "" && buddy.PhoneNumber > "" {
		// keep it globaly in the program for reference
		currentBuddy = buddy
		log.Println(spew.Sprintf("Found a oncall buddy (%s)", buddy))
	} else {
		spew.Errorf("Buddy was empty, should it be ?")
	}

	if currentBuddy != previousBuddy {
		spew.Printf("changed buddy from (%s) to (%s)\n", previousBuddy.Name, currentBuddy.Name)
		BuddyNotificationTask() // notify on change if configured
	}
}

/* BuddyNotificationTask:
 *   Notifies slack webhook url for who is on call
 */
func BuddyNotificationTask() {

	var msg string

	if len(config.SlackWebhookURL) == 0 {
		spew.Printf("SlackWebhookURL is no set, skipping notification\n")
		return
	}

	switch l := len(currentBuddy.Name); {
	case l > 0:
		msg = fmt.Sprintf("<!channel>: Just for your information *%s* (%s) is on call", currentBuddy.Name, currentBuddy.PhoneNumber)
	default:
		msg = "<@channel>: :warning: Error can't find any buddy oncall !\nCheck the calendar!"
	}

	err := SlackNotification(msg, config.SlackWebhookURL, config.SlackChannel)
	if err != nil {
		spew.Printf("WARN: notification hasn't been sent: %s", err)
	}
}

/* main:
 *   Launch a gocron task after setup all access
 */
func main() {

	currentBuddy = OncallBuddy{Name: "", PhoneNumber: ""}

	log.Println("Starting oncall-buddy-finder")
	setup()
	setupGoogleAPIAccess()

	spew.Printf("Configuration: %#v\n", config)

	// setup the http api endpoint
	router := mux.NewRouter()
	spew.Printf("Starting http endpoint %v\n", router)
	go func() {
		log.Fatal(http.ListenAndServe(":8000", router))
	}()
	log.Println("Started http endpoint")
	router.HandleFunc("/buddy", serveCurrentBuddy).Methods("GET")

	// setup the scheduler
	gocron.ChangeLoc(config._Timezone)
	gocron.Every(uint64(config._CheckIntervalDuration) / 1000000000).Seconds().Do(BuddyWatcherTask)
	gocron.Every(config.NotificationInterval).Hours().Do(BuddyNotificationTask)

	// run it once at startup time if configured for
	if config.CheckAtStartup {
		BuddyWatcherTask()
	}

	// schedule other runs
	<-gocron.Start()

	spew.Println("Finished oncall-buddy-finder")
}
