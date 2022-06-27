package main

import (
	// log "github.com/sirupsen/logrus"
	// "gopkg.in/mgo.v2"
	"thingularity/papi/db"
	"thingularity/papi/models"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func getPhysicalDeviceByEUI(EUI string) (model.PhysicalDevice, error) {
	var pd model.PhysicalDevice
	var collection = db.GetDB().C(db.CollPhysicalDevices)

	err := collection.Find(bson.M{"lorawan.EUI": EUI}).One(&pd)

	return pd, err
}

func getPlaceholder(id string) (model.Placeholder, error) {
	var ph model.Placeholder
	var collection = db.GetDB().C(db.CollPlaceholders)

	err := collection.Find(bson.M{"_id": id}).One(&ph)

	return ph, err
}

func updatePhysicalDeviceHW(id string, hw model.HwState) error {
	var collection = db.GetDB().C(db.CollPhysicalDevices)
	var pd model.PhysicalDevice

	change := mgo.Change{
		Update: bson.M{"$set": bson.M{
			"hwstate": hw,
		}},
		ReturnNew: true,
	}
	_, err := collection.Find(bson.M{"_id": id}).Apply(change, &pd)
	return err
}

func updatePhysicalDevice(id string, pd model.PhysicalDevice) error {
	var collection = db.GetDB().C(db.CollPhysicalDevices)

	return collection.UpdateId(id, pd)
}
