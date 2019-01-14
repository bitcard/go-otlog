package otlog

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	encrypt "github.com/tcfw/go-otlog/encrypt"
)

//Operation The operation transformation being applied (e.g. update/delete)
type Operation string

const (
	//OpUpSert the upsert (insert/update) operation
	OpUpSert Operation = "ups"

	//OpDel the delete operation
	OpDel Operation = "del"

	//OpMerge allows for merge operations between other logs
	OpMerge Operation = "merg"
)

//Link provies DAG links/Merkle leaf nodes for IPFS
type Link struct {
	Target string `json:"/"`
}

//Entry holds the DAG struct to go to IPFS
type Entry struct {
	credStore   CredStore
	dataStore   StorageEngine
	isEncrypted bool

	Time       time.Time `json:"t"`
	CrytpoAlg  string    `json:"c"`
	PublicCert string    `json:"pk"`
	Signature  string    `json:"s"`
	Snapshot   *Link     `json:"sn,omitempty"`
	Data       string    `json:"d"`
	Operation  Operation `json:"o"`
	Parent     []*Link   `json:"p,omitempty"`
}

//NewEntry creates a new entry with populated properties
func NewEntry(parent *Link, credStore CredStore, dataStore StorageEngine) (*Entry, error) {
	if credStore.getPass() == "" {
		return nil, errors.New("CredStore must be set with a password")
	}

	return &Entry{
		credStore: credStore,
		dataStore: dataStore,
		Time:      time.Now().Round(0),
		CrytpoAlg: encrypt.AlgoAES256SHA256,
		Parent:    []*Link{parent},
		Operation: OpUpSert,
	}, nil
}

//NewEntryFromStorage gets an entry via storage ref
func NewEntryFromStorage(storage StorageEngine, credStore CredStore, head string) (*Entry, error) {
	entry := &Entry{credStore: credStore, isEncrypted: true}
	entry, err := storage.Get(entry, head)
	if err != nil {
		return nil, err
	}

	err = entry.DecryptData()
	if err != nil {
		return nil, err
	}

	return entry, nil
}

func (e *Entry) parent() (*Entry, error) {
	if len(e.Parent) >= 1 {
		return NewEntryFromStorage(e.dataStore, e.credStore, e.Parent[0].Target)
	}
	return nil, nil
}

//Encrypt alias for EncryptString
func (e *Entry) Encrypt(data string) error {
	return e.EncryptString(data)
}

//EncryptString encrypts & adds data from string
func (e *Entry) EncryptString(data string) error {
	return e.encryptBytes([]byte(data))
}

//EncryptFromJSON takes in a struct, converts to JSON and then encrypts & adds to the entry
func (e *Entry) EncryptFromJSON(data interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return e.encryptBytes(bytes)
}

//DecryptData decrypts and validates data
func (e *Entry) DecryptData() error {
	if !e.isEncrypted {
		return nil
	}

	rawBytes, err := base64.StdEncoding.DecodeString(e.Data)
	if err != nil {
		return err
	}

	dRaw, err := encrypt.Dec(&rawBytes, e.Time, e.credStore.getPass())
	if err != nil {
		return err
	}

	dataString := string(*dRaw)

	_, err = e.validateSignature(dataString)
	if err != nil {
		return err
	}

	e.Data = dataString
	e.isEncrypted = false

	return nil
}

func (e *Entry) validateSignature(data string) (bool, error) {
	decoded, err := base64.StdEncoding.DecodeString(e.PublicCert)
	if err != nil {
		return false, err
	}

	pubCert, err := x509.ParseCertificate(decoded)
	if err != nil {
		return false, err
	}
	//TODO validate public cert against CA

	pubKey := pubCert.PublicKey.(*rsa.PublicKey)

	err = encrypt.Verify(e.Signature, []byte(data), *pubKey)
	if err != nil {
		return false, err
	}

	return true, nil
}

//DataToString decrypts data (if required) and returns as string
func (e *Entry) DataToString() (string, error) {
	if e.isEncrypted {
		err := e.DecryptData()
		if err != nil {
			return "", err
		}
	}

	return e.Data, nil
}

//DataToStruct attempets to converts data to an expected struct
func (e *Entry) DataToStruct(expected interface{}) (interface{}, error) {
	dataString, err := e.DataToString()
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(dataString), expected)
	if err != nil {
		return nil, err
	}

	return expected, nil
}

func (e *Entry) encryptBytes(data []byte) error {
	raw, err := encrypt.Enc(&data, e.Time, e.credStore.getPass())
	if err != nil {
		return err
	}

	sig, err := encrypt.Sign(data, *e.credStore.getPrivKey())
	if err != nil {
		return err
	}

	e.Signature = *sig
	e.PublicCert, err = e.credStore.getPubcert()
	if err != nil {
		return err
	}
	e.Data = base64.StdEncoding.EncodeToString(*raw)
	e.isEncrypted = true

	return nil
}

//Save adds the entry to storage
func (e *Entry) Save(previous string) (string, error) {
	if e.Parent != nil && previous != "" {
		e.Parent = []*Link{{Target: previous}}
	}

	if !e.isEncrypted {
		e.Encrypt(e.Data)
	}

	return e.dataStore.Save(e)
}

//Merge merges 2 entry chains into a single chain
func (e *Entry) Merge(previous *Entry) (*Entry, error) {
	/*
		# Find common base
		# Collate entries between logs into list sorted by time (diff)
		# Walk through changes (priorities delete over upsert)
		# Create snapshot of records
		# Create new entry as merge refing snapshot and both parents
	*/

	return nil, errors.New("not implemented yet")
}
