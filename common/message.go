package common

import (
	"fmt"

	"github.com/IceWhaleTech/CasaOS-LocalStorage/codegen/message_bus"
	"github.com/pilebones/go-udev/netlink"
)

var (
	// devtype -> action -> event
	EventTypes map[string]map[string]message_bus.EventType

	PropertyNameLookupMaps = map[string]map[string]string{
		"disk": {
			fmt.Sprintf("%s:%s", ServiceName, "bus"):    "ID_BUS",
			fmt.Sprintf("%s:%s", ServiceName, "vendor"): "ID_VENDOR",
			fmt.Sprintf("%s:%s", ServiceName, "model"):  "ID_MODEL",
			fmt.Sprintf("%s:%s", ServiceName, "path"):   "DEVNAME",
		},

		"partition": {
			fmt.Sprintf("%s:%s", ServiceName, "bus"):    "ID_BUS",
			fmt.Sprintf("%s:%s", ServiceName, "vendor"): "ID_VENDOR",
			fmt.Sprintf("%s:%s", ServiceName, "model"):  "ID_MODEL",
			fmt.Sprintf("%s:%s", ServiceName, "uuid"):   "ID_FS_UUID", // the only difference
			fmt.Sprintf("%s:%s", ServiceName, "path"):   "DEVNAME",
		},
	}

	ActionPastTense = map[string]string{
		"add":    "added",
		"remove": "removed",
	}
)

func init() {
	// generate event types iteratively, instead of hard coding
	for devtype := range PropertyNameLookupMaps { // devtype = e.g. disk, partition
		for action := range ActionPastTense { // action = e.g. add, remove
			if EventTypes == nil {
				EventTypes = make(map[string]map[string]message_bus.EventType)
			}

			if EventTypes[devtype] == nil {
				EventTypes[devtype] = make(map[string]message_bus.EventType)
			}

			propertyTypeList := make([]message_bus.PropertyType, 0)
			for propertyName := range PropertyNameLookupMaps[devtype] { // propertyName = e.g. local-storage:type
				propertyTypeList = append(propertyTypeList, message_bus.PropertyType{
					Name: propertyName,
				})
			}

			EventTypes[devtype][action] = message_bus.EventType{
				SourceID:         ServiceName,                                                            // e.g. local-storage
				Name:             fmt.Sprintf("%s:%s:%s", ServiceName, devtype, ActionPastTense[action]), // e.g. local-storage:disk:added
				PropertyTypeList: propertyTypeList,
			}
		}
	}

	EventTypes["disk"]["add"] = AddUIPropertyTypes(EventTypes["disk"]["add"])
}

func EventAdapter(e netlink.UEvent) *message_bus.Event {
	devType := e.Env["DEVTYPE"]

	eventType, ok := EventTypes[devType][string(e.Action)]
	if !ok {
		return nil
	}

	properties := make(map[string]string)
	for propertyName, envName := range PropertyNameLookupMaps[devType] {
		value, ok := e.Env[envName]
		if !ok {
			continue
		}
		properties[propertyName] = value

	}

	return &message_bus.Event{
		SourceID:   eventType.SourceID,
		Name:       eventType.Name,
		Properties: properties,
	}
}
