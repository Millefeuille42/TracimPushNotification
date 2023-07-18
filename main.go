package main

import (
	"bytes"
	"encoding/json"
	"github.com/Millefeuille42/TracimDaemonSDK"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

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
		os.Getenv("TRACIM_PUSH_NOTIFICATION_GOTIFY_URL"),
		"application/json",
		bytes.NewBuffer(messageEncoded),
	)

	if err == nil {
		log.Printf("EVENT: %s sent notification\n", conf.Name)
	}

	return err
}

func tracimEventHandler(c *TracimDaemonSDK.TracimDaemonClient, e *TracimDaemonSDK.DaemonEvent) {
	if e.Data == nil {
		log.Print("EVENT: ERROR: no data")
		return
	}

	switch e.Data.(type) {
	case string:
		break
	default:
		log.Print("EVENT: ERROR: Invalid data format")
		return
	}

	eventByte := []byte(e.Data.(string))
	event := TracimDaemonSDK.TLMEvent{}
	err := json.Unmarshal(eventByte, &event)
	if err != nil {
		log.Print("EVENT: ERROR: " + err.Error())
		return
	}

	conf, ok := config[event.EventType]
	if !ok {
		log.Printf("No config for event type %s\n", event.EventType)
		return
	}

	err = sendMessageFromConfig(conf, event.Fields.(map[string]interface{}))
	if err != nil {
		log.Print(err)
	}
}

func loadConfig() {
	configFolder := os.Getenv("TRACIM_PUSH_NOTIFICATION_CONFIG")
	files, err := ioutil.ReadDir(configFolder)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		configData, err := os.ReadFile(configFolder + "/" + file.Name())
		if err != nil {
			log.Print(err)
		}

		rawConfig := NotificationConfigList{}
		err = json.Unmarshal(configData, &rawConfig)
		if err != nil {
			log.Print(err)
			return
		}

		for _, conf := range rawConfig {
			config[conf.EventType] = conf
		}
		log.Printf("Loaded config from %s\n", file.Name())
	}
}

func main() {
	loadConfig()

	client := TracimDaemonSDK.NewClient(TracimDaemonSDK.Config{
		MasterSocketPath: os.Getenv("TRACIM_PUSH_NOTIFICATION_MASTER_SOCKET"),
		ClientSocketPath: os.Getenv("TRACIM_PUSH_NOTIFICATION_SOCKET"),
	})
	_ = os.Remove(client.ClientSocketPath)

	client.HandleCloseOnSig(os.Interrupt)
	err := client.CreateClientSocket()
	if err != nil {
		log.Fatal(err)
		return
	}
	defer client.ClientSocket.Close()

	client.RegisterHandler(TracimDaemonSDK.DaemonTracimEvent, tracimEventHandler)
	err = client.RegisterToMaster()
	if err != nil {
		log.Fatal(err)
		return
	}

	client.ListenToEvents()
}
