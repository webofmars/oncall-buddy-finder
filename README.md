# oncall-buddy-finder

OBF is a tool that help to integrate your monitoring system with a google calendar to manage the oncall shedule within a team.

It will fetch the oncall schedule defined in a google calendar and keep a contact file, based on a template, in sync with the schedule. The contact file is then sourced our notifications system.

The idea is to have this programm running as a service to have always the good contact notified by the monitoring system.

## Templating

The template is totally customizable (handled by golang text/templates)

## Notifications

It also include some notifications capabilities using a slack webhook to inform who is oncall today.

## Requirements

You will need the following:
* a Google Api Service Account Key (from https://console.developers.google.com)
* a config file
* a pre-configured slack webhook (if you want to use notifications)

## Install

```shell
cd src
go get -v -d
go run oncall-buddy-finder.go
```

## Configuration

Configuration is handled as 3 levels (in the order of precedence):

    * JSON config file
    * Env vars
    * default values

The program will try to guess the best config file for you:

    * $CONFIG if ($CONFIG is defined)
    * oncall-buddy-finder.$ENV.json (if $ENV is set)
    * oncall-buddy-finder.json

## Default Values

* GoogleApiSecretFile: "client_secret.json",
* CalendarId: "",
* UserID: "me@example.com",
* TimezoneName: "UTC",
* CheckInterval (in minutes): 60,
* BuddiesList: {},
* SlackWebhookUrl: "",
* SlackChannel: "#general",
* NotificationInterval (in hours): 12,
* FileTemplate: "/etc/oncall-buddy-finder/oncall.vars.tpl",
* FileDest: "/var/run/oncall.vars",

## Contribute

Please feel free to contribute if you think it might be changed in some ways