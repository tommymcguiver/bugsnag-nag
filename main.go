package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/robfig/cron"
	"github.com/tbruyelle/hipchat-go/hipchat"
)

func mustEnv(env string) string {
	e, found := os.LookupEnv(env)

	if !found {
		panic(found)
	}
	return e
}

func getBugsnagToken() string {
	return mustEnv("BUGSNAG_TOKEN")
}

func getHipChatToken() string {
	return mustEnv("HIPCHAT_TOKEN")
}

func getHipChatRoomName() string {
	return mustEnv("HIPCHAT_ROOM_NAME")
}

func getProjectID() string {
	return mustEnv("PROJECT_ID")
}

func getConsoleName() string {
	return mustEnv("CONSOLE_NAME")
}

func formatDate(date time.Time) string {
	return date.UTC().Format("2006-01-02T15:04:05Z")
}

//May not be needed
func formatDateConsole(date time.Time) string {
	//2018-05-04T23 00 00.000Z
	return date.UTC().Format("2006-01-02T15:04:05.000Z")
}

func formatDateReadable(date time.Time) string {
	return date.Format(time.RubyDate)
}

type formatDateFunc func(time.Time) string

func getReportingDates() (start, end time.Time) {

	year, month, day := time.Now().Date()
	today, err := time.Parse(time.RFC3339,
		fmt.Sprintf("%02d-%02d-%02dT09:00:00+10:00", year, month, day))

	if err != nil {
		panic(err)
	}

	s := 2
	e := 1

	//Reporting Periods
	switch today.Weekday() {
	case time.Monday:
		s += 2
		e += 2
	case time.Tuesday:
		s += 2
	}

	start = today.Add(-time.Hour * time.Duration(24*s))
	end = today.Add(-time.Hour * time.Duration(24*e))

	return
}

func generateFilter(f formatDateFunc, start, end time.Time) string {
	return strings.Join(
		[]string{
			"filters[event.since][][type]=eq",
			fmt.Sprintf(
				"filters[event.since][][value]=%s",
				f(start),
			),
			"filters[event.before][][type]=eq",
			fmt.Sprintf(
				"filters[event.before][][value]=%s",
				f(end),
			),
			"filters[error.has_issue][][type]=eq",
			"filters[error.has_issue][][value]=false",
			"filters[error.status][][type]=eq",
			"filters[error.status][][value]=open",
			"filters[error.assigned_to][][type]=ne",
			"filters[error.assigned_to][][value]=anyone",
		},
		"&",
	)
}

var (
	oneShot bool
	verbose bool
)

func init() {
	flag.BoolVar(&oneShot, "oneshot", false, "Check bugsnag immediately")
	flag.BoolVar(&verbose, "verbose", false, "Add verbosity")
	flag.Parse()
}

func logMessage(m string) {
	if verbose {
		log.Println(m)
	}
}

func main() {

	if oneShot {
		checkBugSnag()
		return
	}

	c := cron.New()
	c.ErrorLog = log.New(os.Stderr, "", log.LstdFlags)
	err := c.AddFunc("0 0 9,11 * * MON-FRI", checkBugSnag)
	if err != nil {
		panic(err)
	}

	c.Start()

	//Wait until exit
	signalChan := make(chan os.Signal, 1)
	cleanupDone := make(chan bool)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		for range signalChan {
			cleanupDone <- true
		}
	}()
	<-cleanupDone
}

func checkBugSnag() {

	consoleURL := fmt.Sprintf(
		"https://app.bugsnag.com/%s/errors?",
		getConsoleName(),
	)

	start, end := getReportingDates()

	filters := generateFilter(formatDate, start, end)

	url := []string{
		fmt.Sprintf(
			"https://api.bugsnag.com/projects/%s/errors?",
			getProjectID(),
		),
		filters,
	}

	req, err := http.NewRequest("GET", strings.Join(url, ""), nil)

	if err != nil {
		panic(err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("token %s", getBugsnagToken()))

	client := &http.Client{}

	logMessage("Query bugsnag")

	resp, err := client.Do(req)

	if err != nil {
		panic(err)
	}

	if resp.StatusCode != 200 {
		panic(resp.StatusCode)
	}

	consoleFull := fmt.Sprintf("%s%s", consoleURL,
		strings.Replace(
			strings.Replace(
				strings.Replace(
					generateFilter(formatDateConsole, start, end),
					"[]",
					"[0]",
					-1,
				),
				"[value]",
				"",
				-1,
			),
			"[0]=anyone",
			"[0][value]=anyone",
			1,
		),
	)

	if count := resp.Header.Get("X-Total-Count"); count != "0" {
		//Is a big hack, but filters in console not the same as API

		c := hipchat.NewClient(getHipChatToken())

		notifRq := &hipchat.NotificationRequest{
			Message: fmt.Sprintf(
				"%s bugs not triaged from %s to %s, see: %s",
				count,
				formatDateReadable(start),
				formatDateReadable(end),
				consoleFull,
			),
		}

		logMessage("Notify hipchat")
		_, err := c.Room.Notification(getHipChatRoomName(), notifRq)
		if err != nil {
			panic(err)
		}
		return
	}

	logMessage(fmt.Sprintf(
		"%s bugs not triaged from %s to %s, see: %s",
		"0",
		formatDateReadable(start),
		formatDateReadable(end),
		consoleFull,
	),
	)
}
