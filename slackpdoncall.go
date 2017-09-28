package main

import (
	"encoding/csv"
	"flag"
	"os"
	"time"

	pagerduty "github.com/PagerDuty/go-pagerduty"
	log "github.com/Sirupsen/logrus"
	"github.com/nlopes/slack"
)

type pdClient struct {
	client pagerduty.Client
}

type slackClient struct {
	client slack.Client
}

type pagerMap map[string]string

func (pd pdClient) getOnCalls() ([]pagerduty.OnCall, error) {
	var opts pagerduty.ListOnCallOptions
	var onCalls []pagerduty.OnCall
	for {
		opts.APIListObject.Offset = uint(len(onCalls))
		listOnCallsResponse, err := pd.client.ListOnCalls(opts)
		if err != nil {
			return onCalls, err
		}
		onCalls = append(onCalls, listOnCallsResponse.OnCalls...)
		if !listOnCallsResponse.APIListObject.More {
			break
		}
	}
	return onCalls, nil
}

func (pd pdClient) findOnCallEmail(escalationPolicy string) string {
	onCalls, _ := pd.getOnCalls()

	var onCallID string
	for _, onCall := range onCalls {
		if onCall.EscalationPolicy.Summary == escalationPolicy && onCall.EscalationLevel == 1 {
			onCallID = onCall.User.ID
		}
	}
	var opts pagerduty.GetUserOptions
	onCallUser, err := pd.client.GetUser(onCallID, opts)
	if err != nil {
		log.Fatal("PagerDuty user not found")
	}
	log.WithFields(log.Fields{
		"email":            onCallUser.Email,
		"escalationPolicy": escalationPolicy,
	}).Debug("Found email for escalationPolicy")
	return onCallUser.Email
}

func (slack slackClient) getUserGroupID(handle string) string {
	slackUserGroups, err := slack.client.GetUserGroups()
	if err != nil {
		log.Fatal("Failed to get slack user group list")
	}
	var slackUserGroupID string
	for _, slackUserGroup := range slackUserGroups {
		if slackUserGroup.Handle == handle {
			slackUserGroupID = slackUserGroup.ID
			log.WithFields(log.Fields{
				"handle":           handle,
				"slackUserGroupID": slackUserGroupID,
			}).Debug("Found slackUserGroupID for handle")
		}
	}
	return slackUserGroupID
}

func (slack slackClient) getUserID(email string) string {
	slackUsers, err := slack.client.GetUsers()
	if err != nil {
		log.Fatal("Failed to get slack user list")
	}
	var slackUserID string
	for _, slackUser := range slackUsers {
		if slackUser.Profile.Email == email {
			slackUserID = slackUser.ID
			log.WithFields(log.Fields{
				"email":       email,
				"slackUserID": slackUserID,
			}).Debug("Found slackUserID for email")
		}
	}
	return slackUserID
}

func readSyncMap(file string) (map[string]string, error) {
	f, err := os.Open(file)
	defer f.Close()
	if err != nil {
		return nil, err
	}
	onCallMap := make(map[string]string)
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	for _, pair := range records {
		onCallMap[pair[0]] = pair[1]
	}
	return onCallMap, nil
}

func main() {
	var debug = flag.Bool("debug", false, "output debugging information")
	var noOp = flag.Bool("noop", false, "only print actions")
	var syncInterval = flag.Int("interval", -1, "seconds to wait between sync loops")
	var syncMap = flag.String("map", "", "csv file containing pagerduty to slack mapping (required)")
	flag.Parse()
	if *syncMap == "" {
		flag.Usage()
		os.Exit(2)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	onCallMap, err := readSyncMap(*syncMap)
	if err != nil {
		log.Fatalf("Error reading map from %s", *syncMap)
	}

	pdToken := os.Getenv("PD_TOKEN")
	if pdToken == "" {
		log.Fatal("PD_TOKEN must be set")
	}
	var pdAPI pdClient
	pdAPI.client = *pagerduty.NewClient(pdToken)

	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		log.Fatal("SLACK_TOKEN must be set")
	}
	var sAPI slackClient
	sAPI.client = *slack.New(slackToken)

	for {
		for escalationPolicy, userGroup := range onCallMap {
			log.WithFields(log.Fields{
				"escalationPolicy": escalationPolicy,
				"userGroup":        userGroup,
			}).Info("Syncing escalationPolicy and userGroup")

			onCallEmail := pdAPI.findOnCallEmail(escalationPolicy)
			slackUserGroupID := sAPI.getUserGroupID(userGroup)
			slackUserID := sAPI.getUserID(onCallEmail)

			if *noOp {
				log.WithFields(log.Fields{
					"slackUserID":      slackUserID,
					"slackUserGroupID": slackUserGroupID,
				}).Info("Would assign slackUserID to slackUserGroupID")
			} else {
				_, err := sAPI.client.UpdateUserGroupMembers(slackUserGroupID, slackUserID)
				if err != nil {
					log.WithError(err).Fatal("Failed to update user group")
				}
			}
		}
		if *syncInterval == -1 {
			os.Exit(0)
		} else {
			time.Sleep(time.Duration(*syncInterval) * time.Second)
		}
	}
}
