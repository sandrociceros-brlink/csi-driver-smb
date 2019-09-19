package credentials

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"os"

	"github.com/pborman/uuid"
	"github.com/pelletier/go-toml"

	"k8s.io/klog"
)

const (
	AzurePublicCloud            = "AzurePublicCloud"
	AzureChinaCloud             = "AzureChinaCloud"
	TempAzureCredentialFilePath = "/tmp/azure.json"

	azureCredentialFileTemplate = `{
    "cloud": "{{.Cloud}}",
    "tenantId": "{{.TenantID}}",
    "subscriptionId": "{{.SubscriptionID}}",
    "aadClientId": "{{.AADClientID}}",
    "aadClientSecret": "{{.AADClientSecret}}",
    "resourceGroup": "{{.ResourceGroup}}",
    "location": "{{.Location}}"
}`
	defaultAzurePublicCloudLocation = "eastus2"
	defaultAzureChinaCloudLocation  = "chinaeast2"
)

// CredentialsConfig is used in Prow to store Azure credentials
// https://github.com/kubernetes/test-infra/blob/master/kubetest/utils/azure.go#L116-L118
type CredentialsConfig struct {
	Creds CredentialsFromProw
}

// CredentialsFromProw is used in Prow to store Azure credentials
// https://github.com/kubernetes/test-infra/blob/master/kubetest/utils/azure.go#L107-L114
type CredentialsFromProw struct {
	ClientID           string
	ClientSecret       string
	TenantID           string
	SubscriptionID     string
	StorageAccountName string
	StorageAccountKey  string
}

// Credentials is used in Azure File CSI Driver to store Azure credentials
type Credentials struct {
	Cloud           string
	TenantID        string
	SubscriptionID  string
	AADClientID     string
	AADClientSecret string
	ResourceGroup   string
	Location        string
}

// CreateAzureCredentialFile creates a temporary Azure credential file for
// Azure File CSI driver tests and returns the credentials
func CreateAzureCredentialFile(isAzureChinaCloud bool) (*Credentials, error) {
	// Search credentials through env vars first
	var cloud, tenantId, subscriptionId, aadClientId, aadClientSecret, resourceGroup, location string
	if isAzureChinaCloud {
		cloud = AzureChinaCloud
		tenantId = os.Getenv("tenantId_china")
		subscriptionId = os.Getenv("subscriptionId_china")
		aadClientId = os.Getenv("aadClientId_china")
		aadClientSecret = os.Getenv("aadClientSecret_china")
		resourceGroup = os.Getenv("resourceGroup_china")
		location = os.Getenv("location_china")
	} else {
		cloud = AzurePublicCloud
		tenantId = os.Getenv("tenantId")
		subscriptionId = os.Getenv("subscriptionId")
		aadClientId = os.Getenv("aadClientId")
		aadClientSecret = os.Getenv("aadClientSecret")
		resourceGroup = os.Getenv("resourceGroup")
		location = os.Getenv("location")
	}

	if resourceGroup == "" {
		resourceGroup = "azurefile-csi-driver-test-" + uuid.NewUUID().String()
	}

	if location == "" {
		if isAzureChinaCloud {
			location = defaultAzureChinaCloudLocation
		} else {
			location = defaultAzurePublicCloudLocation
		}
	}

	if tenantId != "" && subscriptionId != "" && aadClientId != "" && aadClientSecret != "" {
		return parseAndExecuteTemplate(cloud, tenantId, subscriptionId, aadClientId, aadClientSecret, resourceGroup, location)
	}

	// If the tests are being run on Prow, credentials are not supplied through env vars. Instead, it is supplied
	// through env var AZURE_CREDENTIALS. We need to convert it to AZURE_CREDENTIAL_FILE for sanity, integration and E2E tests
	// https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes/cloud-provider-azure/cloud-provider-azure-config.yaml#L5-L6
	if azureCredentialsPath, ok := os.LookupEnv("AZURE_CREDENTIALS"); ok {
		klog.V(2).Infof("Running in Prow, converting AZURE_CREDENTIALS to AZURE_CREDENTIAL_FILE")
		c, err := getCredentialsFromAzureCredentials(azureCredentialsPath)
		if err != nil {
			return nil, err
		}
		// We only test on AzurePublicCloud in Prow
		return parseAndExecuteTemplate(cloud, c.TenantID, c.SubscriptionID, c.ClientID, c.ClientSecret, resourceGroup, location)
	}

	return nil, fmt.Errorf("AZURE_CREDENTIALS is not set. You will need to set the following env vars: $tenantId, $subscriptionId, $aadClientId and $aadClientSecret")
}

// CreateAzureCredentialFile deletes the temporary Azure credential file
func DeleteAzureCredentialFile() error {
	if err := os.Remove(TempAzureCredentialFilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing %s %v", TempAzureCredentialFilePath, err)
	}

	return nil
}

// getCredentialsFromAzureCredentials parses the azure credentials toml (AZURE_CREDENTIALS)
// in Prow and return the credential information usable to Azure File CSI driver
func getCredentialsFromAzureCredentials(azureCredentialsPath string) (*CredentialsFromProw, error) {
	content, err := ioutil.ReadFile(azureCredentialsPath)
	klog.V(2).Infof("Reading credentials file %v", azureCredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("error reading credentials file %v %v", azureCredentialsPath, err)
	}

	c := CredentialsConfig{}
	if err := toml.Unmarshal(content, &c); err != nil {
		return nil, fmt.Errorf("error parsing credentials file %v %v", azureCredentialsPath, err)
	}

	return &c.Creds, nil
}

// parseAndExecuteTemplate replaces credential placeholders in hack/template/azure.json with actual credentials
func parseAndExecuteTemplate(cloud, tenantId, subscriptionId, aadClientId, aadClientSecret, resourceGroup, location string) (*Credentials, error) {
	t := template.New("AzureCredentialFileTemplate")
	t, err := t.Parse(azureCredentialFileTemplate)
	if err != nil {
		return nil, fmt.Errorf("error parsing  azureCredentialFileTemplate %v", err)
	}

	f, err := os.Create(TempAzureCredentialFilePath)
	if err != nil {
		return nil, fmt.Errorf("error creating %s %v", TempAzureCredentialFilePath, err)
	}
	defer f.Close()

	c := Credentials{
		cloud,
		tenantId,
		subscriptionId,
		aadClientId,
		aadClientSecret,
		resourceGroup,
		location,
	}
	err = t.Execute(f, c)
	if err != nil {
		return nil, fmt.Errorf("error executing parsed azure credential file tempalte %v", err)
	}

	return &c, nil
}
