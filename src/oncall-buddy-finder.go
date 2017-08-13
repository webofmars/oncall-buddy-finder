// Copyright 2017 - webofmars - frederic leger
package main

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "encoding/json"
    "io/ioutil"
    "path/filepath"
    "time"
    calendar "google.golang.org/api/calendar/v3"
    "golang.org/x/net/context"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
    "github.com/jasonlvhit/gocron"
    "text/template"
    "github.com/tkanos/gonfig"
    "github.com/ashwanthkumar/slack-go-webhook"
    "github.com/davecgh/go-spew/spew"
    "strings"
)

// globals
type Configuration struct {
  CalendarId string
  GoogleApiTokenFile string
  GoogleApiSecretFile string
  TimezoneName string
  CheckInterval uint64
  BuddiesList map[string]string
  FileTemplate string
  FileDest string
  SlackWebhookUrl string
  SlackChannel string
  NotificationInterval uint64
}

var buddiesList map[string]string

var config Configuration = Configuration{
  GoogleApiTokenFile: "/var/run/oncall-buddy-finder/google_token.json",
  GoogleApiSecretFile: "/etc/oncall-buddy-finder/client_secret.json",
  CalendarId: "",
  TimezoneName: "UTC",
  CheckInterval: 60,
  BuddiesList: buddiesList,
  FileTemplate: "/etc/oncall-buddy-finder/oncall.vars.tpl",
  FileDest: "/var/run/oncall-buddy-finder/oncall.vars",
  SlackWebhookUrl: "",
  SlackChannel: "#general",
  NotificationInterval: 12,
}

var ctx = context.Background()
var client *http.Client
var timezone *time.Location = time.UTC
var currentBuddy string

// TODO: in case of buddy not found should notify a warning !
// TODO: should not fail if no config file and should rely on env & defaults
// TODO: implement a sanity check
// TODO: config file should be passed on the command line instead of env ?
// TODO: a Buddy should be a struct : Buddy.Name, Buddy.Phone
// TODO: instead of looking on 24hours, make that configurable
// TODO: How to get the phone numbers through the env variables ?
// TODO: would love to get rid of google oauth token if possible
// TODO: scheduler init should be more flexible (arrays of times, start at certain time, etc...)

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
    "authorization code /!\\: \n%v\n", authURL)

  var code string
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
  timezone = getTimezone(config.TimezoneName)
}

// try to guess the config file to use (based on $CONFIG or $ENV)
// if it can't will return a default one
func GuessConfigurationFilename() (filename string) {
  var forced string = os.Getenv("CONFIG")
  var env string    = os.Getenv("ENV")

  // passed through $CONFIG
  if (len(forced) > 0) {
    _, err := os.Stat(forced)
    if (err == nil) {
      return forced
    }
    // this is fatal because if this is forced we must trust it, no ?
    log.Fatalf("ERROR: can't load %s config file, skipping\n", forced)
  }

  // CWD through $ENV
  if (len(env) > 0) {
    path, err := filepath.Abs("oncall-buddy-finder." + env + ".json")
    _, err = os.Stat(path)
    if (err == nil) {
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
  if (err != nil) {
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

// gat all the Google Calendar events
func getCalendarEvents(calendarId string, TimeMin time.Time, TimeMax time.Time, MaxResults int64) (*calendar.Events, error) {

  svc, err := calendar.New(client)
  if err != nil {
    log.Printf("Error: Unable to create Calendar service: %v", err)
    return nil, err
  }

  log.Printf("TimeMin : %s\n", TimeMin.Format(time.RFC3339))
  log.Printf("TimeMax : %s\n", TimeMax.Format(time.RFC3339))

  // get the calendars events
  events, err := svc.Events.List(config.CalendarId).
                            TimeMin(TimeMin.Format(time.RFC3339)).
                            TimeMax(TimeMax.Format(time.RFC3339)).
                            MaxResults(MaxResults).
                            Do();

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
        log.Printf("entry: %s - start: %s\n", i.Summary, i.Start.Date)
      }
    }
}

// find the name of the current buddy who is oncall
func getCurrentBuddyName() (buddy string) {
  // get the timeframe
  tmin  := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), int(0),int(0),int(0),int(0), timezone)
  tmax  := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), int(23),int(59),int(59),int(0), timezone)

  events, err := getCalendarEvents(config.CalendarId, tmin, tmax, 1)
  if err != nil {
    log.Printf("Error: Unable to fetch events from calendar : %v", err)
    return ""
  }

  // print the result
  printEventsInfos(events)

  if len(events.Items) > 0 {
      for _, i := range events.Items {
        if i.Summary != "" { buddy = strings.ToLower(i.Summary) }
        break
      }
  }

  return
}

// find the phone number of the current buddy who is oncall
func getCurrentBuddyPhone(name string) (phoneNumber string) {
  phoneNumber, exists := config.BuddiesList[name]
  if !exists {
    log.Printf("ERROR: %s phone number is not known in the buddies list: %s", name, config.BuddiesList)
    return
  }
  return
}

// generates the contact file
func renderContactTemplate(buddyName string, buddyPhoneNumber string, templateFile string, outputFile string) (bool, error) {
  tpl, err := template.ParseFiles(templateFile)
  if (err != nil) {
    log.Printf("Error: Unable to parse template file: %v", err)
    return false, err
  }

  f, err := os.OpenFile(config.FileDest, os.O_RDWR|os.O_CREATE, 0644)
  if (err != nil) {
    log.Printf("Error: Unable to parse template file: %v", err)
    return false, err
  }

  data := struct {
    Name string
    Phone string
  }{ Name: buddyName, Phone: buddyPhoneNumber}

  err = tpl.Execute(f, data )
  if (err != nil) {
    log.Printf("Error: Unable to render template file: %v", err)
    return false, err
  }

  return true, nil
}

// notify the current buddy by slack
func SlackNotification(msg string, slackWebhookUrl string, slackChannelName string) ([]error) {

  payload := slack.Payload {
      Text: msg,
      Channel: slackChannelName,
      LinkNames: "1",
  }

  err := slack.Send(slackWebhookUrl, "", payload)
  if len(err) > 0 {
    log.Printf("ERROR: Unable to send slack notification: %s\n", err)
    return err
  }
  return nil
}

// the main task that will be executed in loop
func BuddyWatcherTask() {
  var name string = getCurrentBuddyName()
  if (name != "") {

    // get Phone Number from buddies list
    var tel string  = getCurrentBuddyPhone(name)
    log.Printf("Will go with Phone Number : %s\n", tel)

    // keep it globaly in the program for reference
    currentBuddy = name

    // render the template with the infos
    _, err := renderContactTemplate(name, tel, config.FileTemplate, config.FileDest)
    if (err != nil) {
      log.Printf("Can't render contact file: %s\n", err)
    }

  } else {
    log.Printf("Buddy was empty, should it be ?")
  }
}

// a notification task to be sure who is on call
func BuddyNotificationTask() {

  var msg string = ""

  if (len(config.SlackWebhookUrl) == 0) {
    log.Printf("SlackWebhookUrl is no set, skipping notification\n")
    return
  }

  switch l := len(currentBuddy); {
  case l > 0:
    msg = fmt.Sprintf("<!channel>: Just for your information *%s* is on call", currentBuddy)
  default:
    msg = "<@channel>: :warning: Error can't find any buddy oncall !\nCheck the calendar!"
  }

  err := SlackNotification(msg, config.SlackWebhookUrl, config.SlackChannel)
  if (err != nil) {
    log.Printf("WARN: notification hasn't been sent: %s", err)
  }
}

// victory : the main function
// launch a gocron task after setup
func main() {

    log.Println("starting buddy finder")
    setup()
    setupGoogleApiAccess()

    spew.Printf("configuration: %#v\n", config)

    // setup the scheduler
    gocron.ChangeLoc(getTimezone(config.TimezoneName))
    gocron.Every(config.CheckInterval).Minute().Do(BuddyWatcherTask)
    gocron.Every(config.NotificationInterval).Hours().Do(BuddyNotificationTask)

    // run it once at startup time
    gocron.RunAll()

    // schedule other runs
    <- gocron.Start()

    log.Println("finished buddy finder")
}
