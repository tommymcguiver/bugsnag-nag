package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tbruyelle/hipchat-go/hipchat"
)

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

func generateFilter(f formatDateFunc, today time.Time) string {
	return strings.Join(
		[]string{
			"filters[event.since][][type]=eq",
			fmt.Sprintf(
				"filters[event.since][][value]=%s",
				f(today.Add(-time.Hour*24*2)),
			),
			"filters[event.before][][type]=eq",
			fmt.Sprintf(
				"filters[event.before][][value]=%s",
				f(today.Add(-time.Hour*24*1)),
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

func MustEnv(env string) string {
	e, found := os.LookupEnv(env)

	if !found {
		panic(found)
	}
	return e
}

func getBugsnagToken() string {
	return MustEnv("BUGSNAG_TOKEN")
}

func getHipChatToken() string {
	return MustEnv("HIPCHAT_TOKEN")
}

func getHipChatRoomName() string {
	return MustEnv("HIPCHAT_ROOM_NAME")
}

func getProjectID() string {
	return MustEnv("PROJECT_ID")
}

func getConsoleName() string {
	return MustEnv("CONSOLE_NAME")
}

func main() {

	consoleURL := fmt.Sprintf(
		"https://app.bugsnag.com/%s/errors?",
		getConsoleName(),
	)

	year, month, day := time.Now().Date()
	today, err := time.Parse(time.RFC3339,
		fmt.Sprintf("%02d-%02d-%02dT09:00:00+10:00", year, month, day))

	if err != nil {
		panic(err)
	}

	filters := generateFilter(formatDate, today)

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

	resp, err := client.Do(req)

	if err != nil {
		panic(err)
	}

	if resp.StatusCode != 200 {
		panic(resp.StatusCode)
	}

	if count := resp.Header.Get("X-Total-Count"); count != "0" {
		//Is a big hack, but filters in console not the same as API
		consoleFull := fmt.Sprintf("%s%s", consoleURL,
			strings.Replace(
				strings.Replace(
					strings.Replace(
						generateFilter(formatDateConsole, today),
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

		c := hipchat.NewClient(getHipChatToken())

		notifRq := &hipchat.NotificationRequest{
			Message: fmt.Sprintf(
				"%s bugs not triaged from %s to %s, see: %s",
				count,
				formatDateReadable(today.Add(-time.Hour*24*2)),
				formatDateReadable(today.Add(-time.Hour*24*1)),
				consoleFull,
			),
		}

		_, err := c.Room.Notification(getHipChatRoomName(), notifRq)
		if err != nil {
			panic(err)
		}
	}
}
