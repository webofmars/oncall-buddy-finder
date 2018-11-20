# gscalendar

Retrieves 10 (or less) upcoming events from G Suite user's (USER_ID) calendar (CALENDAR_ID) using service account credentials (SERVICE_ACCOUNT_FILE_PATH).

## Usage

```sh
go build -o gscalendar .
SERVICE_ACCOUNT_FILE_PATH=key.json USER_ID=user@domain.com CALENDAR_ID=primary ./gscalendar
```

## how to setup service account

1. Go to [https://admin.google.com/ManageOauthClients](https://admin.google.com/ManageOauthClients) and:

    * paste your service account "Unique ID" into "Client Name" field;
    * paste `https://www.googleapis.com/auth/calendar.events.readonly` (without quotes) into "One or More API Scopes"
    * click "Authorize" button

2. Go to [https://console.developers.google.com/iam-admin/serviceaccounts](https://console.developers.google.com/iam-admin/serviceaccounts) :

    * select a project
    * click on email of your service account
    * click on "Edit" button
    * create new key as JSON
    * use this key as credentials for your app