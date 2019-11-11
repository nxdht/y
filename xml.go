package y

import (
	"encoding/xml"
	"io/ioutil"
)

func LoadXml(fileName string, cfg interface{}) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	err = xml.Unmarshal(data, &cfg)
	if err != nil {
		return err
	}
	return nil
}

func SaveXml(fileName string, cfg interface{}) error {
	data, err := xml.Marshal(cfg)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fileName, data, 0777)
}
