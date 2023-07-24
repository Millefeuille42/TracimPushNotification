package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Millefeuille42/Daemonize"
	"github.com/Millefeuille42/TracimDaemonSDK"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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

var daemonizer *Daemonize.Daemonizer = nil
var globalNotificationConfig = make([]NotificationConfig, 0)
var logMutex = sync.Mutex{}

func safeLog(severity Daemonize.Severity, v ...any) {
	logMutex.Lock()
	defer logMutex.Unlock()
	if daemonizer == nil {
		log.Print(v...)
		return
	}
	daemonizer.Log(severity, v...)
}

func errorLogger(c *TracimDaemonSDK.TracimDaemonClient, e *TracimDaemonSDK.DaemonEvent) {
	err := TracimDaemonSDK.ParseDaemonData(e, &TracimDaemonSDK.TypeErrorData{})
	if err != nil {
		safeLog(Daemonize.LOG_ERR, err)
		return
	}

	safeLog(Daemonize.LOG_ERR, e.Data.(*TracimDaemonSDK.TypeErrorData).Error)
}

func getPropertyFromKey(key string, fields map[string]interface{}) string {
	keys := strings.Split(key, ".")
	if len(keys) == 1 {
		if fields[key] == nil {
			return "<invalid>"
		}
		return fmt.Sprintf("%v", fields[key])
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
			safeLog(Daemonize.LOG_INFO, fmt.Sprintf("EVENT: %s filtered out (%s) - %s", conf.Name, filter.Name, value))
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
		globalConfig.GotifyUrl,
		"application/json",
		bytes.NewBuffer(messageEncoded),
	)

	if err == nil {
		safeLog(Daemonize.LOG_INFO, fmt.Sprintf("EVENT: %s sent notification", conf.Name))
	}

	return err
}

func tracimEventHandler(c *TracimDaemonSDK.TracimDaemonClient, e *TracimDaemonSDK.DaemonEvent) {
	safeLog(Daemonize.LOG_INFO, fmt.Sprintf("EVENT: RECV: %s", e.Type))

	if e.Data == nil {
		safeLog(Daemonize.LOG_ERR, "EVENT: ERROR: no data")
		return
	}

	switch e.Data.(type) {
	case string:
		break
	default:
		safeLog(Daemonize.LOG_ERR, "EVENT: ERROR: Invalid data format")
		return
	}

	eventByte := []byte(e.Data.(string))
	event := TracimDaemonSDK.TLMEvent{}
	err := json.Unmarshal(eventByte, &event)
	if err != nil {
		safeLog(Daemonize.LOG_ERR, "EVENT: ERROR: "+err.Error())
		return
	}

	for _, conf := range globalNotificationConfig {
		if conf.EventType == event.EventType {
			err = sendMessageFromConfig(conf, event.Fields.(map[string]interface{}))
			if err != nil {
				safeLog(Daemonize.LOG_ERR, err)
			}
		}
	}
}

func loadConfig() {
	configFolder := globalConfig.NotificationConfigFolder
	files, err := ioutil.ReadDir(configFolder)
	if err != nil {
		safeLog(Daemonize.LOG_EMERG, err)
	}

	for _, file := range files {
		configData, err := os.ReadFile(configFolder + "/" + file.Name())
		if err != nil {
			safeLog(Daemonize.LOG_ERR, err)
		}

		rawConfig := NotificationConfigList{}
		err = json.Unmarshal(configData, &rawConfig)
		if err != nil {
			safeLog(Daemonize.LOG_ERR, err)
			return
		}

		for _, conf := range rawConfig {
			globalNotificationConfig = append(globalNotificationConfig, conf)
		}
		safeLog(Daemonize.LOG_INFO, fmt.Sprintf("Loaded config from %s", file.Name()))
	}
}

func startProcess() {
	loadConfig()
	defer os.Remove(globalConfig.SocketPath)
	_ = os.Remove(globalConfig.SocketPath)

	client := TracimDaemonSDK.NewClient(TracimDaemonSDK.Config{
		MasterSocketPath: globalConfig.MasterSocketPath,
		ClientSocketPath: globalConfig.SocketPath,
	})
	defer client.Close()

	err := client.CreateClientSocket()
	if err != nil {
		safeLog(Daemonize.LOG_EMERG, err)
		os.Exit(1)
	}
	defer client.ClientSocket.Close()

	client.Logger = func(v ...any) { safeLog(Daemonize.LOG_INFO, v...) }
	client.RegisterHandler(TracimDaemonSDK.DaemonTracimEvent, tracimEventHandler)
	client.RegisterHandler(TracimDaemonSDK.EventTypeError, errorLogger)
	err = client.RegisterToMaster()
	if err != nil {
		safeLog(Daemonize.LOG_EMERG, err)
		os.Exit(1)
	}

	client.ListenToEvents()
}

func main() {
	setGlobalConfig()

	if len(os.Args) > 1 && os.Args[1] == "-p" {
		startProcess()
		os.Exit(0)
	}

	var err error = nil
	daemonizer, err = Daemonize.NewDaemonizer()
	if err != nil {
		log.Fatal(err)
		return
	}
	defer daemonizer.Close()

	pid, err := daemonizer.Daemonize(nil)
	if err != nil {
		log.Fatal(err)
	}
	if pid != 0 {
		log.Print(pid)
		os.Exit(0)
	}

	pattern := fmt.Sprintf("push_%s_*.log", time.Now().Format(time.RFC3339))
	err = daemonizer.AddTempFileLogger(configDir+"log", pattern, os.Args[0], log.LstdFlags)
	if err != nil {
		log.Fatal(err)
		return
	}

	startProcess()
}
