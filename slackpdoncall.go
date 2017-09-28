package main

import (
	"os"
	"strings"
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
	}).Info("Found email for escalationPolicy")
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
			}).Info("Found slackUserGroupID for handle")
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
			}).Info("Found slackUserID for email")
		}
	}
	return slackUserID
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s pd:slack[,pd:slack]", os.Args[0])
	}

	syncInterval := 60

	onCallMap := make(map[string]string)
	for _, pair := range strings.Split(os.Args[1], ",") {
		s := strings.Split(pair, ":")
		onCallMap[s[0]] = s[1]
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
			onCallEmail := pdAPI.findOnCallEmail(escalationPolicy)
			slackUserGroupID := sAPI.getUserGroupID(userGroup)
			slackUserID := sAPI.getUserID(onCallEmail)

			log.WithFields(log.Fields{
				"slackUserID":      slackUserID,
				"slackUserGroupID": slackUserGroupID,
			}).Info("Will assign slackUserID to slackUserGroupID")

			// _, err := sAPI.UpdateUserGroupMembers(slackUserGroupID, slackUserID)
			// if err != nil {
			// 	log.WithError(err).Fatal("Failed to update user group")
			// }
		}
		time.Sleep(time.Duration(syncInterval) * time.Second)
	}
}
