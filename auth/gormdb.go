package auth

import (
	"errors"
	"log"
	"time"

	"github.com/panyam/goutils/utils"
	"gorm.io/gorm"
)

type AuthDB struct {
	storage *gorm.DB
}

func NewAuthDB(gormdb *gorm.DB) *AuthDB {
	gormdb.AutoMigrate(&Channel{})
	gormdb.AutoMigrate(&Identity{})

	return &AuthDB{storage: gormdb}
}

func (adb *AuthDB) SaveChannel(entity *Channel) (err error) {
	entity.UpdatedAt = time.Now()
	result := adb.storage.Save(entity)
	err = result.Error
	if result.Error == nil && result.RowsAffected == 0 {
		entity.CreatedAt = time.Now()
		err = adb.storage.Create(entity).Error
	}
	return
}

func (adb *AuthDB) GetChannel(provider string, loginId string) (*Channel, error) {
	var out Channel
	err := adb.storage.First(&out, "provider = ? AND login_id = ?", provider, loginId).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	return &out, nil
}

func (adb *AuthDB) EnsureChannel(provider string, loginId string, params utils.StrMap) (*Channel, bool) {
	channel, _ := adb.GetChannel(provider, loginId)
	newCreated := channel == nil
	if channel == nil {
		channel = NewChannel(provider, loginId, params)
	}
	channel.Credentials = params["credentials"].(utils.StrMap)
	channel.Profile = params["profile"].(utils.StrMap)
	if err := adb.SaveChannel(channel); err != nil {
		log.Println("Error saving channel: ", err)
	}
	return channel, newCreated
}

func (adb *AuthDB) GetIdentity(idType string, idKey string) (*Identity, error) {
	var out Identity
	err := adb.storage.First(&out, "identity_type = ? AND identity_key = ?", idType, idKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else {
			return nil, err
		}
	}
	return &out, nil
}

func (adb *AuthDB) SaveIdentity(entity *Identity) (err error) {
	entity.UpdatedAt = time.Now()
	result := adb.storage.Save(entity)
	err = result.Error
	if result.Error == nil && result.RowsAffected == 0 {
		entity.CreatedAt = time.Now()
		err = adb.storage.Create(entity).Error
	}
	return
}

func (adb *AuthDB) EnsureIdentity(idType string, idKey string, params utils.StrMap) (*Identity, bool) {
	identity, _ := adb.GetIdentity(idType, idKey)
	newCreated := identity == nil
	if identity == nil {
		identity = NewIdentity(idType, idKey)
	}
	if err := adb.SaveIdentity(identity); err != nil {
		log.Println("Error saving identity: ", err)
	}
	return identity, newCreated
}
