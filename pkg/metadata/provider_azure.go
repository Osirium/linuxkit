package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"time"
)

const (
	// metadata URL for Azure
	azureMetaDataURL = "http://169.254.169.254/metadata/instance/%s?api-version=2019-06-04&format=text"
)

// ProviderAzure is the type implementing the Provider interface for Azure
type ProviderAzure struct {
}

// NewAzure returns a new ProviderAzure
func NewAzure() *ProviderAzure {
	return &ProviderAzure{}
}

func (p *ProviderAzure) String() string {
	return "Azure"
}

// Probe checks if we are running on Azure
func (p *ProviderAzure) Probe() bool {
	// Getting the public ipv4 should always work...
	_, err := azureGet(fmt.Sprintf(azureMetaDataURL, "network/interface/0/ipv4/ipAddress/0/publicIpAddress"))
	return (err == nil)
}

// Extract gets both the Azure specific and generic userdata
func (p *ProviderAzure) Extract() ([]byte, error) {
	// Get public ipv4. This must not fail
	publicIPAddress, err := azureGet(fmt.Sprintf(azureMetaDataURL, "network/interface/0/ipv4/ipAddress/0/publicIpAddress"))
	if err != nil {
		return nil, err
	}
	err = ioutil.WriteFile(path.Join(ConfigPath, "public_ipv4"), publicIPAddress, 0644)
	if err != nil {
		return nil, fmt.Errorf("Azure: Failed to write public IP address: %s", err)
	}
	// private ipv4
	azureMetaGet("network/interface/0/ipv4/ipAddress/0/privateIpAddress", "local_ipv4", 0644)

	// availability zone
	azureMetaGet("compute/zone", "availability_zone", 0644)

	// instance type
	azureMetaGet("compute/vmSize", "instance_type", 0644)

	// instance-id
	azureMetaGet("compute/vmId", "instance_id", 0644)

	// ssh
	if err := p.handleSSH(); err != nil {
		log.Printf("Azure: Failed to get ssh data: %s", err)
	}

	// currently there is no API for userdata in Azure
	return nil, nil
}

// lookup a value (lookupName) in azure metaservice and store in given fileName
func azureMetaGet(lookupName string, fileName string, fileMode os.FileMode) {
	if lookupValue, err := azureGet(fmt.Sprintf(azureMetaDataURL, lookupName)); err == nil {
		// we got a value from the metadata server, now save to filesystem
		err = ioutil.WriteFile(path.Join(ConfigPath, fileName), lookupValue, fileMode)
		if err != nil {
			// we couldn't save the file for some reason
			log.Printf("Azure: Failed to write %s:%s %s", fileName, lookupValue, err)
		}
	} else {
		// we did not get a value back from the metadata server
		log.Printf("Azure: Failed to get %s: %s", lookupName, err)
	}
}

// azureGet requests and extracts the requested URL
func azureGet(url string) ([]byte, error) {
	var client = &http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest("", url, nil)
	req.Header.Add("Metadata", "true")
	if err != nil {
		return nil, fmt.Errorf("Azure: http.NewRequest failed: %s", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Azure: Could not contact metadata service: %s", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Azure: Status not ok: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Azure: Failed to read http response: %s", err)
	}
	return body, nil
}

// SSH keys:
func (p *ProviderAzure) handleSSH() error {
	sshKeys, err := azureGet(fmt.Sprintf(azureMetaDataURL, "compute/publicKeys/0/keyData"))
	if err != nil {
		return fmt.Errorf("Failed to get sshKeys: %s", err)
	}

	if err := os.Mkdir(path.Join(ConfigPath, SSH), 0755); err != nil {
		return fmt.Errorf("Failed to create %s: %s", SSH, err)
	}

	err = ioutil.WriteFile(path.Join(ConfigPath, SSH, "authorized_keys"), sshKeys, 0600)
	if err != nil {
		return fmt.Errorf("Failed to write ssh keys: %s", err)
	}
	return nil
}
