# slackpdsync

Using a mapping of PagerDuty escalation policy name to Slack user group handle from a CSV file, create a dynamic on-call alias.

### CSV format

```
Production Engineering,prodeng
Database Admins,dba
```

### Usage

By default, one sync operation will run and then exit. An `-interval` flag may be provided to sleep and continue syncing. Supply `-help` to see additional usage information.
