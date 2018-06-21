// Copyright 2017 - webofmars - frederic leger
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/davecgh/go-spew/spew"
	"github.com/jasonlvhit/gocron"
	"github.com/tkanos/gonfig"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
)

// globals vars

// Configuration : gonfig struct
type Configuration struct {
	CalendarId             string
	GoogleApiTokenFile     string
	GoogleApiSecretFile    string
	TimezoneName           string
	CheckAtStartup         bool
	CheckFirstAt           string
	CheckInterval          string
	BuddiesList            map[string]string
	FileTemplate           string
	FileDest               string
	SlackWebhookUrl        string
	SlackChannel           string
	NotificationInterval   uint64
	_CheckIntervalDuration time.Duration
	_Timezone              *time.Location
}

// oncall buddy struct
type OncallBuddy struct {
	Name        string
	PhoneNumber string
}

var OncallBuddies map[string]string

var config Configuration = Configuration{
	GoogleApiTokenFile:     "/var/run/oncall-buddy-finder/google_token.json",
	GoogleApiSecretFile:    "/etc/oncall-buddy-finder/client_secret.json",
	CalendarId:             "",
	TimezoneName:           "UTC",
	CheckAtStartup:         true,
	CheckFirstAt:           "08:00",
	CheckInterval:          "60s",
	BuddiesList:            OncallBuddies,
	FileTemplate:           "/etc/oncall-buddy-finder/oncall.vars.tpl",
	FileDest:               "/var/run/oncall-buddy-finder/oncall.vars",
	SlackWebhookUrl:        "",
	SlackChannel:           "#general",
	NotificationInterval:   12,
	_CheckIntervalDuration: 128,
	_Timezone:              time.UTC,
}

var ctx = context.Background()
var client *http.Client
var currentBuddy OncallBuddy

// TODO: should not fail if no config file and should rely on env & defaults
// TODO: implement a sanity check
// TODO: config file should be passed on the command line instead of env ?
// TODO: How to get the phone numbers through the env variables ?
// TODO: would love to get rid of google oauth token if possible

/********************************************************************
 * Functions
 ********************************************************************/

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("/!\\ Go to the following link in your browser then type the "+
		"authorization code /!\\: \n\n%v\n", authURL)

	var code string
	fmt.Print("Enter your code>")

	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	return filepath.Abs(config.GoogleApiTokenFile)
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// set some vars & load config file based on env values
func setup() {

	spew.Config.Indent = "\t"

	var configFilePath = GuessConfigurationFilename()
	log.Printf("INFO: Using %s config file", configFilePath)
	LoadConfiguration(configFilePath, &config)
	config._Timezone = getTimezone(config.TimezoneName)
	config._CheckIntervalDuration = getCheckIntervalDuration(config.CheckInterval)
}

// GuessConfigurationFilename :
// 	try to guess the config file to use (based on $CONFIG or $ENV)
// 	if it can't will return a default one
func GuessConfigurationFilename() (filename string) {
	var forced string = os.Getenv("CONFIG")
	var env string = os.Getenv("ENV")

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
		log.Printf("WARN: can't load %s config file, skipping\n", path)
	}

	return "oncall-buddy-finder.json"
}

// load the config from guessed config file path (and keeps defaults)
/* the order is the following:
 * - defaults values
 * - config file values
 * - env values
 */
func LoadConfiguration(configFilePath string, config *Configuration) {

	err := gonfig.GetConf(configFilePath, config)
	if err != nil {
		log.Fatalf("Can't load configuration file: %s\n", err)
	}
}

// setup a correct connexion to google API
func setupGoogleApiAccess() {
	b, err := ioutil.ReadFile(config.GoogleApiSecretFile)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client = getClient(ctx, config)
}

// load correct Timezone based on the name provided
func getTimezone(timezoneStr string) (timezone *time.Location) {

	timezone, err := time.LoadLocation(timezoneStr)

	if err != nil {
		log.Fatalf("Unable to load timezone")
	}
	return
}

// getCheckIntervalDuration : Parse the configuration check interval (a string) into time.Duration
func getCheckIntervalDuration(interval string) time.Duration {
	d1, err := time.ParseDuration(config.CheckInterval)
	if err != nil {
		spew.Errorf("Can't parse time interval %s", interval)
	}
	return d1
}

// getCalendarEvents : get all the Google Calendar events
func getCalendarEvents(calendarID string, TimeMin time.Time, TimeMax time.Time, TimeZone *time.Location, MaxResults int64) (*calendar.Events, error) {

	svc, err := calendar.New(client)
	if err != nil {
		log.Printf("Error: Unable to create Calendar service: %v", err)
		return nil, err
	}

	spew.Printf("TimeMin : %s\n", TimeMin.Format(time.RFC3339))
	spew.Printf("TimeMax : %s\n", TimeMax.Format(time.RFC3339))

	// get the calendars events
	events, err := svc.Events.List(config.CalendarId).
		TimeZone(TimeZone.String()).
		TimeMin(TimeMin.Format(time.RFC3339)).
		TimeMax(TimeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Do()

	if err != nil {
		log.Printf("Error: Unable to get calendar events: %v", err)
		return nil, err
	}

	return events, err
}

// prints events (used for debug mostly)
func printEventsInfos(events *calendar.Events) {
	if len(events.Items) > 0 {
		for _, i := range events.Items {
			log.Printf("entry: %s - start: %v\n", i.Summary, i.Start.DateTime)
		}
	}
}

// find the name of the current buddy who is oncall
func getCurrentBuddy(interval time.Duration) (buddy OncallBuddy, err error) {
	spew.Printf("interval : %s\n", interval)

	// get the timeframe
	tmin := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), time.Now().Hour(), time.Now().Minute(), int(00), int(0), time.Now().Location())
	tmax := tmin.Add(interval)

	// get the claendar events for this timeframe
	events, err := getCalendarEvents(config.CalendarId, tmin, tmax, config._Timezone, 1)
	if err != nil {
		log.Printf("Error: Unable to fetch events from calendar : %v", err)
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

// find the phone number of the current buddy who is oncall
func getCurrentBuddyPhone(name string) (phoneNumber string, err error) {
	phoneNumber, exists := config.BuddiesList[name]
	if !exists {
		err = fmt.Errorf("ERROR: %s phone number is not known in the buddies list: %s", name, config.BuddiesList)
		return
	}
	return
}

// generates the contact file
func renderContactTemplate(buddy OncallBuddy, templateFile string, outputFile string) (bool, error) {
	tpl, err := template.ParseFiles(templateFile)
	if err != nil {
		fmt.Errorf("Error: Unable to parse template file: %v", err)
		return false, err
	}

	f, err := os.OpenFile(config.FileDest, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Printf("Error: Unable to parse template file: %v", err)
		return false, err
	}

	data := struct {
		Name  string
		Phone string
	}{Name: buddy.Name, Phone: buddy.PhoneNumber}

	err = tpl.Execute(f, data)
	if err != nil {
		log.Printf("Error: Unable to render template file: %v", err)
		return false, err
	}

	return true, nil
}

// SlackNotification : notify the current buddy by slack
func SlackNotification(msg string, slackWebhookUrl string, slackChannelName string) []error {

	payload := slack.Payload{
		Text:      msg,
		Channel:   slackChannelName,
		LinkNames: "1",
	}

	err := slack.Send(slackWebhookUrl, "", payload)
	if len(err) > 0 {
		log.Printf("ERROR: Unable to send slack notification: %s\n", err)
		return err
	}
	return nil
}

// BuddyWatcherTask : The main task that will be executed in loop
func BuddyWatcherTask() {

	var err error
	var previousBuddy OncallBuddy
	previousBuddy = currentBuddy

	buddy, err := getCurrentBuddy(config._CheckIntervalDuration)
	if err != nil {
		spew.Errorf("Can't retrieve curreng buddy")
	}

	if buddy.Name > "" && buddy.PhoneNumber > "" {
		// keep it globaly in the program for reference
		currentBuddy = buddy
		log.Println(spew.Sprintf("Found a oncall buddy (%s)", buddy))

		// render the template with the infos
		_, err := renderContactTemplate(buddy, config.FileTemplate, config.FileDest)
		if err != nil {
			spew.Errorf("Can't render contact file: %s\n", err)
		}
	} else {
		spew.Errorf("Buddy was empty, should it be ?")
	}

	if currentBuddy != previousBuddy {
		log.Printf("changed buddy from (%s) to (%s)", previousBuddy.Name, currentBuddy.Name)
		BuddyNotificationTask() // we want to notify on change, no ?
	}
}

// BuddyNotificationTask : notifies slack webhook url for who is on call
func BuddyNotificationTask() {

	var msg string

	if len(config.SlackWebhookUrl) == 0 {
		log.Printf("SlackWebhookUrl is no set, skipping notification\n")
		return
	}

	switch l := len(currentBuddy.Name); {
	case l > 0:
		msg = fmt.Sprintf("<!channel>: Just for your information *%s* (%s) is on call", currentBuddy.Name, currentBuddy.PhoneNumber)
	default:
		msg = "<@channel>: :warning: Error can't find any buddy oncall !\nCheck the calendar!"
	}

	err := SlackNotification(msg, config.SlackWebhookUrl, config.SlackChannel)
	if err != nil {
		log.Printf("WARN: notification hasn't been sent: %s", err)
	}
}

// victory : the main function
// launch a gocron task after setup
func main() {

	log.Println("Starting oncall-buddy-finder")
	setup()
	setupGoogleApiAccess()

	spew.Printf("Configuration: %#v\n", config)

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
