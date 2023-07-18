package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type configuration struct {
	NotificationConfigFolder string `json:"notification_config_folder"`
	SocketPath               string `json:"socket_path"`
	MasterSocketPath         string `json:"master_socket_path"`
	GotifyUrl                string `json:"gotify_url"`
}

func createDirIfNotExist(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.Mkdir(path, os.ModePerm)
		}
	}
	return err
}

func createFileIfNotExist(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			_, err = os.Create(path)
		}
	}
	return err
}

func isExist(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func createDefaultConfig(path string) (*configuration, error) {
	defaultConfig := configuration{
		NotificationConfigFolder: path + "notifications/",
		SocketPath:               path + "tracim_push_notification.sock",
		MasterSocketPath:         "",
		GotifyUrl:                "",
	}

	configBytes, err := json.MarshalIndent(defaultConfig, "", "\t")
	if err != nil {
		return nil, err
	}

	err = createFileIfNotExist(path + "config.json")
	if err != nil {
		return nil, err
	}

	err = createDirIfNotExist(defaultConfig.NotificationConfigFolder)
	if err != nil {
		return nil, err
	}

	return &defaultConfig, os.WriteFile(path+"config.json", configBytes, os.ModePerm)
}

func getConfigDir() (string, error) {
	var dir = ""
	var err error = nil

	if len(os.Args) > 1 {
		dir = os.Args[1]
	} else {
		dir, err = os.UserConfigDir()
		if err != nil {
			dir, err = os.UserHomeDir()
			if err != nil {
				return dir, err
			}
			dir = dir + "./config"
			err = createDirIfNotExist(dir)
		}
	}

	return dir, err
}

func readConfigFromFile() (*configuration, error) {
	dir, err := getConfigDir()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/TracimPushNotification/", dir)
	err = createDirIfNotExist(path)
	if err != nil {
		return nil, err
	}

	exist, err := isExist(path + "config.json")
	if err != nil {
		return nil, err
	}

	if !exist {
		return createDefaultConfig(path)
	}

	configBytes, err := os.ReadFile(path + "config.json")
	if err != nil {
		return nil, err
	}

	conf := configuration{}
	err = json.Unmarshal(configBytes, &conf)
	return &conf, err
}

func setGlobalConfig() {
	newConf, err := readConfigFromFile()
	if err != nil {
		log.Fatal(err)
		return
	}
	globalConfig = *newConf
}

var globalConfig configuration
