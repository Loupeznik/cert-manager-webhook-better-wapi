package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&betterWapiDNSProviderSolver{},
	)
}

type betterWapiDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type betterWapiDNSProviderConfig struct {
	BaseURL             string                    `json:"baseUrl"`
	UserLoginSecretRef  cmmeta.SecretKeySelector `json:"userLoginSecretRef"`
	UserSecretSecretRef cmmeta.SecretKeySelector `json:"userSecretSecretRef"`
}

type authRequest struct {
	Login  string `json:"login"`
	Secret string `json:"secret"`
}

type authResponse struct {
	Token string `json:"token"`
}

type recordRequest struct {
	Autocommit bool   `json:"autocommit"`
	Data       string `json:"data"`
	Subdomain  string `json:"subdomain"`
	TTL        int    `json:"ttl"`
	Type       string `json:"type"`
}

func (c *betterWapiDNSProviderSolver) Name() string {
	return "better-wapi"
}

func (c *betterWapiDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	userLogin, err := c.getSecret(cfg.UserLoginSecretRef, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("error getting user login secret: %v", err)
	}

	userSecret, err := c.getSecret(cfg.UserSecretSecretRef, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("error getting user secret secret: %v", err)
	}

	domain := extractDomain(ch.ResolvedFQDN)
	subdomain := extractSubdomain(ch.ResolvedFQDN, domain)

	token, err := c.authorize(cfg.BaseURL, userLogin, userSecret)
	if err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	if err := c.createRecord(cfg.BaseURL, token, domain, subdomain, ch.Key); err != nil {
		return fmt.Errorf("failed to create DNS record: %v", err)
	}

	return nil
}

func (c *betterWapiDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	userLogin, err := c.getSecret(cfg.UserLoginSecretRef, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("error getting user login secret: %v", err)
	}

	userSecret, err := c.getSecret(cfg.UserSecretSecretRef, ch.ResourceNamespace)
	if err != nil {
		return fmt.Errorf("error getting user secret secret: %v", err)
	}

	domain := extractDomain(ch.ResolvedFQDN)
	subdomain := extractSubdomain(ch.ResolvedFQDN, domain)

	token, err := c.authorize(cfg.BaseURL, userLogin, userSecret)
	if err != nil {
		return fmt.Errorf("authorization failed: %v", err)
	}

	if err := c.deleteRecord(cfg.BaseURL, token, domain, subdomain, ch.Key); err != nil {
		return fmt.Errorf("failed to delete DNS record: %v", err)
	}

	return nil
}

func (c *betterWapiDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return fmt.Errorf("error creating kubernetes client: %v", err)
	}

	c.client = cl
	return nil
}

func (c *betterWapiDNSProviderSolver) authorize(baseURL, login, secret string) (string, error) {
	authReq := authRequest{
		Login:  login,
		Secret: secret,
	}

	body, err := json.Marshal(authReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth request: %v", err)
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/api/auth/token", strings.TrimSuffix(baseURL, "/")),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth request failed with status: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read auth response: %v", err)
	}

	var authResp authResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal auth response: %v", err)
	}

	return authResp.Token, nil
}

func (c *betterWapiDNSProviderSolver) createRecord(baseURL, token, domain, subdomain, challengeKey string) error {
	record := recordRequest{
		Autocommit: true,
		Data:       challengeKey,
		Subdomain:  subdomain,
		TTL:        300,
		Type:       "TXT",
	}

	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record request: %v", err)
	}

	url := fmt.Sprintf("%s/api/v1/domain/%s/record", strings.TrimSuffix(baseURL, "/"), domain)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("create record request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create record failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *betterWapiDNSProviderSolver) deleteRecord(baseURL, token, domain, subdomain, challengeKey string) error {
	record := recordRequest{
		Autocommit: true,
		Data:       challengeKey,
		Subdomain:  subdomain,
		TTL:        300,
		Type:       "TXT",
	}

	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record request: %v", err)
	}

	url := fmt.Sprintf("%s/api/v1/domain/%s/record", strings.TrimSuffix(baseURL, "/"), domain)
	req, err := http.NewRequest(http.MethodDelete, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete record request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete record failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *betterWapiDNSProviderSolver) getSecret(ref cmmeta.SecretKeySelector, namespace string) (string, error) {
	secret, err := c.client.CoreV1().Secrets(namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %v", namespace, ref.Name, err)
	}

	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s/%s", ref.Key, namespace, ref.Name)
	}

	return string(value), nil
}

func extractDomain(fqdn string) string {
	tldPattern := regexp.MustCompile(`\.([^.]+\.[^.]+)\.$`)
	matches := tldPattern.FindStringSubmatch(fqdn)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func extractSubdomain(fqdn, domain string) string {
	subdomain := strings.TrimSuffix(fqdn, "."+domain+".")
	return subdomain
}

func loadConfig(cfgJSON *extapi.JSON) (betterWapiDNSProviderConfig, error) {
	cfg := betterWapiDNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
