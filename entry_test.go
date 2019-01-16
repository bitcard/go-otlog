package otlog

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	ipfsShell "github.com/ipfs/go-ipfs-shell"
	"github.com/stretchr/testify/assert"
)

var TestPass = hex.EncodeToString([]byte(`abcdefhigKLMNOPQRSTUVWXYZ_123456`))

func generateTestKeys() (*rsa.PrivateKey, *x509.Certificate, error) {

	privKey, _ := rsa.GenerateKey(rand.Reader, 1024)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		log.Fatalf("failed to generate serial number: %s", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "test._.example.com.clog.com",
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return nil, nil, err
	}

	cert, _ := x509.ParseCertificate(certBytes)
	return privKey, cert, nil

}

func generateTestCredStore() *CredStore {
	privKey, pubCert, _ := generateTestKeys()
	encryptor, _ := NewCredStore(TestPass, *privKey, *pubCert)

	return encryptor
}

func TestNewEntry(t *testing.T) {
	privKey, pubCert, err := generateTestKeys()
	if err != nil {
		t.Fatal(err)
	}

	encryptor, err := NewCredStore(TestPass, *privKey, *pubCert)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %s", err.Error())
	}

	_, err = NewEntry(nil, *encryptor, &IpfsStore{})
	if err != nil {
		t.Fatal(err)
	}

	//Empty CredStore
	encryptor = &CredStore{}
	_, err = NewEntry(nil, *encryptor, &IpfsStore{})
	if err == nil {
		t.Fatal("Must pick up that no pass exists")
	}
}

func TestEncryptString(t *testing.T) {
	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	err := entry.EncryptString("test")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Data == "test" {
		t.Fatal("Encrypted data still matches original data")
	}
	if entry.Signature == "" {
		t.Fatal("Signure failed")
	}
	if entry.PublicCert == "" {
		t.Fatal("Pub Cert not attached")
	}
}

func TestDecryptString(t *testing.T) {
	origData := `test`

	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	err := entry.Encrypt(origData)
	if err != nil {
		t.Fatal(err)
	}

	err = entry.DecryptData()
	if err != nil {
		t.Fatal(err)
	}

	//Double decrypt
	err = entry.DecryptData()
	if err != nil {
		t.Fatal(err)
	}

	if entry.Data != origData {
		t.Fatal("Original data is lost")
	}
}

func TestInvalidSigs(t *testing.T) {
	origData := `test`

	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	invalids := []struct {
		Sig  string
		Desc string
	}{
		{Sig: "2", Desc: "Invalid base64"},
		{Sig: base64.StdEncoding.EncodeToString([]byte("2")), Desc: "Invalid sig"},
	}

	for _, invalid := range invalids {
		t.Run(fmt.Sprintf("Invalid sig: %s", invalid.Desc), func(t *testing.T) {
			err := entry.Encrypt(origData)
			if err != nil {
				t.Fatal(err)
			}

			entry.Signature = invalid.Sig

			_, err = entry.validateSignature(origData)
			if err == nil {
				t.Fatal("Should have failed")
			}
		})
	}
}

func TestInvalidPubCerts(t *testing.T) {
	origData := `test`
	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	invalids := []struct {
		Cert string
		Desc string
	}{
		{Cert: "2", Desc: "Invalid base64"},
		{Cert: base64.StdEncoding.EncodeToString([]byte("2")), Desc: "Invalid cert"},
	}

	for _, invalid := range invalids {
		t.Run(fmt.Sprintf("Invalid sig: %s", invalid.Desc), func(t *testing.T) {
			err := entry.Encrypt(origData)
			if err != nil {
				t.Fatal(err)
			}

			entry.PublicCert = invalid.Cert

			_, err = entry.validateSignature(origData)
			if err == nil {
				t.Fatal("Should have failed")
			}
		})
	}
}

func TestInvalidEncData(t *testing.T) {
	origData := `test`
	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	invalids := []struct {
		Data string
		Desc string
	}{
		{Data: "2", Desc: "Invalid base64"},
		{Data: base64.StdEncoding.EncodeToString([]byte("2")), Desc: "Invalid cert"},
	}

	for _, invalid := range invalids {
		t.Run(fmt.Sprintf("Invalid sig: %s", invalid.Desc), func(t *testing.T) {
			err := entry.Encrypt(origData)
			if err != nil {
				t.Fatal(err)
			}

			entry.Data = invalid.Data
			err = entry.DecryptData()
			if err == nil {
				t.Fatal("Should have failed")
			}
		})
	}

}

func TestDataToString(t *testing.T) {
	origData := `test`
	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})

	err := entry.EncryptString(origData)
	if err != nil {
		t.Fatal(err)
	}

	nData, err := entry.DataToString()
	if err != nil {
		t.Fatal(err)
	}
	if nData != origData {
		t.Fatal("Original data lost")
	}
}

func TestEncryptStruct(t *testing.T) {
	basicStruct := &struct {
		Name string `json:"name"`
	}{
		Name: "Test",
	}

	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{})
	err := entry.EncryptFromJSON(basicStruct)
	if err != nil {
		t.Fatal(err)
	}

	compStructDef := &struct {
		Name string `json:"name"`
	}{}

	_, err = entry.DataToStruct(compStructDef)
	if err != nil {
		t.Fatal(err)
	}

	if basicStruct.Name != compStructDef.Name {
		t.Fatal("Data Comparison failed")
	}

}

func TestIPFSSave(t *testing.T) {
	shell := ipfsShell.NewShell("localhost:5001")
	origData := `test`
	entry, _ := NewEntry(nil, *generateTestCredStore(), &IpfsStore{Shell: shell})

	nTime, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	if err != nil {
		t.Fatal(err)
	}
	entry.Time = nTime
	entry.EncryptString(origData)

	//Remove authenticators otherwise will always result in a different hash
	entry.PublicCert = ""
	entry.Signature = ""
	entry.ID = uuid.Nil

	expectedHead := "zdpuAwEQfhajYWLX8nk6XmW3cjFtNDAQtMHkZTEgc6mrZHPxN"

	head, err := entry.Save("")
	if err != nil {
		t.Fatal(err)
	}
	if head != expectedHead {
		t.Fatalf("Unexpected head, should have been (%s) but got (%s)", expectedHead, head)
	}
}

func TestNewEntryFromIPFS(t *testing.T) {
	encryptor := generateTestCredStore()

	shell := ipfsShell.NewShell("localhost:5001")
	origData := `test`
	entry, _ := NewEntry(nil, *encryptor, &IpfsStore{Shell: shell})
	entry.EncryptString(origData)

	head, _ := entry.Save("")
	t.Log("Head at ", head, " Data: ", entry.Data)
	entry.DecryptData()

	ipfsEntry, err := NewEntryFromStorage(&IpfsStore{Shell: shell}, *encryptor, head)
	if err != nil {
		t.Fatal(err)
	}

	assert.EqualValues(t, entry.Data, ipfsEntry.Data)
}

func TestFindDirectAncestor(t *testing.T) {
	/*
	   	Test:

	   		root
	   		 /\
	   		/  \
	     entry 1  entry 2

	   	LCA(entry1, entry2): root
	*/

	memStore := NewMemStore()
	credStore := generateTestCredStore()

	root, _ := NewEntry(nil, *credStore, memStore)
	rootRef, _ := root.Save("")

	entry1, _ := NewEntry(&Link{rootRef}, *credStore, memStore)
	entry1.Save("")

	entry2, _ := NewEntry(&Link{rootRef}, *credStore, memStore)
	entry2.Save("")

	_, estRootRef, _, _ := entry2.findCommonAncestor(entry1)

	assert.Equal(t, rootRef, *estRootRef)
}

func TestFind1JumpAncestor(t *testing.T) {
	/*
	   	Test:

	   		root
	   		  /\
	   	     /  \
	    entry 1  entry 2
	   	 |
	   	 |
	    entry 3

	   	LCA(entry3, entry2) = root

	*/
	memStore := NewMemStore()
	credStore := *generateTestCredStore()

	root, _ := NewEntry(nil, credStore, memStore)
	rootRef, _ := root.Save("")

	entry1, _ := NewEntry(&Link{rootRef}, credStore, memStore)
	entry1Ref, _ := entry1.Save("")

	entry3, _ := NewEntry(&Link{entry1Ref}, credStore, memStore)
	entry3.Save("")

	entry2, _ := NewEntry(&Link{rootRef}, credStore, memStore)
	entry2.Save("")

	_, estRootRef, _, _ := entry2.findCommonAncestor(entry3)

	if estRootRef == nil {
		t.Fatal("No ancestor found, but should have been ", rootRef)
	}

	assert.Equal(t, rootRef, *estRootRef)
}

func TestFind2JumpAncestor(t *testing.T) {
	/*
		   	Test:

		   		root
		   		  /\
		   	     /  \
		    entry 1  entry 2
		   	 |	   \	|
		   	 |		 \	|
			entry 3  entry 4
						|
						|
					 entry 5

		   	LCA(entry3, entry5) = entry1

	*/
	memStore := NewMemStore()
	credStore := *generateTestCredStore()

	root, _ := NewEntry(nil, credStore, memStore)
	rootRef, _ := root.Save("")

	entry1, _ := NewEntry(&Link{rootRef}, credStore, memStore)
	entry1Ref, _ := entry1.Save("")

	entry3, _ := NewEntry(&Link{entry1Ref}, credStore, memStore)
	entry3Ref, _ := entry3.Save("")

	entry2, _ := NewEntry(&Link{rootRef}, credStore, memStore)
	entry2Ref, _ := entry2.Save("")

	entry4, _ := NewEntry(nil, credStore, memStore)
	entry4.Parent = []*Link{{entry1Ref}, {entry2Ref}}
	entry4Ref, _ := entry4.Save("")

	entry5, _ := NewEntry(nil, credStore, memStore)
	entry5.Parent = []*Link{{entry4Ref}}
	entry5Ref, _ := entry5.Save("")

	t.Logf("\nroot: %s\n1: %s\n2: %s\n3: %s\n4: %s\n5: %s\n", rootRef, entry1Ref, entry2Ref, entry3Ref, entry4Ref, entry5Ref)

	_, estRootRef, _, _ := entry3.findCommonAncestor(entry5)

	if estRootRef == nil {
		t.Fatal("No ancestor found, but should have been ", rootRef)
	}

	assert.Equal(t, entry1Ref, *estRootRef)
}

type basicTestRecord struct {
	_id  uuid.UUID
	name string
}

func TestSimpleMerge(t *testing.T) {
	/*
	   	Test:

	   		root
	   		 /\
	   		/  \
	  entry 1  entry 2

	   	LCA(entry1, entry2): root
	*/

	memStore := NewMemStore()
	credStore := generateTestCredStore()

	root, _ := NewEntry(nil, *credStore, memStore)
	root.Operation = OpBase
	rootRef, _ := root.Save("")

	entry1, _ := NewEntry(&Link{rootRef}, *credStore, memStore)
	entry1.Operation = OpUpSert
	rec1 := &Record{uuid.Nil, []byte(`"Test"`)}
	entry1Recs := &Records{Records: []Record{*rec1}}
	entry1Diff := &EntryDiff{OpUpSert, *rec1}
	entry1Snap, _ := NewSnapshot(*credStore, entry1Recs, memStore)
	entry1.Snapshot = entry1Snap
	entry1.EncryptFromJSON(entry1Diff)
	entry1Ref, _ := entry1.Save("")

	entry2, _ := NewEntry(&Link{rootRef}, *credStore, memStore)
	entry2.Operation = OpUpSert
	rec2 := &Record{uuid.Nil, []byte(`"Example"`)}
	entry2Recs := &Records{Records: []Record{*rec2}}
	entry2Diff := &EntryDiff{OpUpSert, *rec2}
	entry2Snap, _ := NewSnapshot(*credStore, entry2Recs, memStore)
	entry2.Snapshot = entry2Snap
	entry2.EncryptFromJSON(entry2Diff)
	entry2Ref, _ := entry2.Save("")

	merge, mRecs, err := entry1.Merge(entry2)
	if err != nil {
		t.Fatal(err)
	}
	parents, err := merge.Parents()
	if err != nil {
		t.Fatal(err)
	}
	pRefs := []string{}
	for k := range parents {
		pRefs = append(pRefs, k)
	}

	expectedRefs := []string{entry1Ref, entry2Ref}
	expectedRecords := []Record{*rec1, *rec2}

	assert.EqualValues(t, expectedRefs, pRefs)
	assert.Equal(t, OpMerge, merge.Operation)
	assert.EqualValues(t, mRecs, expectedRecords)
}
