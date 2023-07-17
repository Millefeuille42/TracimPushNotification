package main

import (
	"bytes"
	"encoding/json"
	"github.com/Millefeuille42/TracimDaemonSDK"
	"log"
	"net/http"
	"os"
	"strings"
)

var testJson = `
[
  {
    "name": "reaction_created_comment",
	"event_type": "reaction.created",
	"elements": [
      {
        "name": "reaction_author_username",
        "key": "reaction.author.username"
      },
      {
        "name": "reaction_value",
        "key": "reaction.value"
      },
      {
        "name": "comment_name",
        "key": "content.parent_label"
      }
    ],
    "filters" : [
	  {
	    "name": "content_type",
		"key": "content.content_type",
        "match": "equal",
		"value": "comment"
      },
	  {
	    "name": "users_content",
		"key": "content.author.username",
        "match": "equal",
		"value": "millefeuille"
      }
    ],
    "notification": {
      "title": "Tracim reaction ({{reaction_author_username}})",
      "message": "{{reaction_author_username}} reacted {{reaction_value}} on your comment: {{comment_name}}",
      "priority": 5
    }
  }
]
`

type GotifyMessage struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

type TLMElements struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type TLMFilters struct {
	TLMElements
	Match string `json:"match"`
	Value string `json:"value"`
}

type NotificationConfig struct {
	Name         string        `json:"name"`
	EventType    string        `json:"event_type"`
	Elements     []TLMElements `json:"elements"`
	Filters      []TLMFilters  `json:"filters"`
	Notification GotifyMessage `json:"notification"`
}

type NotificationConfigList []NotificationConfig

var config = make(map[string]NotificationConfig)

func getPropertyFromKey(key string, fields map[string]interface{}) string {
	keys := strings.Split(key, ".")
	if len(keys) == 1 {
		if fields[key] == nil {
			return "<invalid>"
		}
		return fields[key].(string)
	}

	return getPropertyFromKey(strings.Join(keys[1:], "."), fields[keys[0]].(map[string]interface{}))
}

func applyMatch(match string, value string, filterValue string) bool {
	ret := false
	invert := strings.HasPrefix(match, "not_")
	match = strings.ReplaceAll(strings.ToLower(match), "not_", "")
	switch match {
	case "equal":
		ret = value == filterValue
	case "contains":
		ret = strings.Contains(value, filterValue)
	case "starts_with":
		ret = strings.HasPrefix(value, filterValue)
	case "ends_with":
		ret = strings.HasSuffix(value, filterValue)
	}
	if invert {
		return !ret
	}
	return ret
}

func sendMessageFromConfig(conf NotificationConfig, fields map[string]interface{}) error {
	message := conf.Notification

	for _, filter := range conf.Filters {
		value := getPropertyFromKey(filter.Key, fields)
		if !applyMatch(filter.Match, value, filter.Value) {
			log.Printf("EVENT: %s filtered out (%s)\n", conf.Name, filter.Name)
			return nil
		}
	}

	for _, element := range conf.Elements {
		value := getPropertyFromKey(element.Key, fields)
		message.Message = strings.ReplaceAll(message.Message, "{{"+element.Name+"}}", value)
		message.Title = strings.ReplaceAll(message.Title, "{{"+element.Name+"}}", value)
	}

	messageEncoded, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = http.Post(
		os.Getenv("TRACIM_MINICLIENT_GOTIFY_URL"),
		"application/json",
		bytes.NewBuffer(messageEncoded),
	)

	if err == nil {
		log.Printf("EVENT: %s sent notification\n", conf.Name)
	}

	return err
}

func genericHandler(c *TracimDaemonSDK.TracimDaemonClient, e *TracimDaemonSDK.Event) {
	log.Printf("RECV: %s\n", e.DataParsed.EventType)

	conf, ok := config[e.DataParsed.EventType]
	if !ok {
		log.Printf("No config for event type %s\n", e.DataParsed.EventType)
		return
	}

	err := sendMessageFromConfig(conf, e.DataParsed.Fields.(map[string]interface{}))
	if err != nil {
		log.Print(err)
	}
}

func loadConfig() {
	rawConfig := NotificationConfigList{}
	err := json.Unmarshal([]byte(testJson), &rawConfig)
	if err != nil {
		log.Print(err)
		return
	}

	for _, conf := range rawConfig {
		config[conf.EventType] = conf
	}
}

func main() {
	loadConfig()

	client := TracimDaemonSDK.NewClient(TracimDaemonSDK.Config{
		MasterSocketPath: os.Getenv("TRACIM_MINICLIENT_MASTER_SOCKET_PATH"),
		ClientSocketPath: os.Getenv("TRACIM_MINICLIENT_CLIENT_SOCKET_PATH"),
	})
	_ = os.Remove(client.ClientSocketPath)

	client.HandleCloseOnSig(os.Interrupt)
	err := client.CreateClientSocket()
	if err != nil {
		log.Fatal(err)
		return
	}
	defer client.ClientSocket.Close()

	client.RegisterHandler(TracimDaemonSDK.EventTypeGeneric, genericHandler)
	err = client.RegisterToMaster()
	if err != nil {
		log.Fatal(err)
		return
	}

	client.ListenToEvents()
}
