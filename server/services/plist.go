package services

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// xmlTextEscaper escapes the characters that are significant inside an XML
// text node. Sync keys are typically opaque base64-ish tokens, but we escape
// defensively since the value is attacker-influenced configuration, not a
// compile-time constant.
var xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// GenerateMobileConfig generates an Apple Configuration Profile (.mobileconfig)
// for easy Santa configuration deployment. When syncKey is non-empty it is
// embedded as a SyncExtraHeaders X-Santa-Key entry so that reinstalling the
// profile carries the sync key to the client.
func GenerateMobileConfig(machineID, clientMode, syncBaseURL, organizationName, machineOwner string, uploadInterval int, syncKey string) string {
	// Convert client mode to integer
	mode := 1
	if clientMode == "LOCKDOWN" {
		mode = 2
	}

	// Ensure syncBaseURL uses HTTPS
	if strings.HasPrefix(syncBaseURL, "http://") {
		syncBaseURL = strings.Replace(syncBaseURL, "http://", "https://", 1)
	} else if !strings.HasPrefix(syncBaseURL, "https://") {
		syncBaseURL = "https://" + syncBaseURL
	}

	// Generate UUIDs for the profile and payload
	profileUUID := uuid.New().String()
	payloadUUID := uuid.New().String()

	if organizationName == "" {
		organizationName = "Krampus Santa Sync"
	}

	// Default machine owner if not provided
	if machineOwner == "" {
		machineOwner = "admin"
	}

	// Staged rollout: when a sync key is configured, push it to the client
	// via Santa's SyncExtraHeaders so the key is sent with every sync request.
	syncExtraHeaders := ""
	if syncKey != "" {
		syncExtraHeaders = fmt.Sprintf(`
			<key>SyncExtraHeaders</key>
			<dict>
				<key>X-Santa-Key</key>
				<string>%s</string>
			</dict>`, xmlTextEscaper.Replace(syncKey))
	}

	mobileConfigTemplate := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Santa Configuration</string>
			<key>PayloadDisplayName</key>
			<string>Santa</string>
			<key>PayloadIdentifier</key>
			<string>com.northpolesec.santa.%s</string>
			<key>PayloadOrganization</key>
			<string>%s</string>
			<key>PayloadType</key>
			<string>com.northpolesec.santa</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>SyncBaseURL</key>
			<string>%s</string>
			<key>ClientMode</key>
			<integer>%d</integer>
			<key>MachineID</key>
			<string>%s</string>
			<key>MachineOwner</key>
			<string>%s</string>
			<key>FullSyncInterval</key>
			<integer>%d</integer>
			<key>EventLogType</key>
			<string>json</string>
			<key>EnableAllEventUpload</key>
			<true/>
			<key>EnablePageZeroProtection</key>
			<true/>
			<key>EnableBundles</key>
			<true/>
			<key>EnableTransitiveRules</key>
			<false/>
			<key>BannedBlockMessage</key>
			<string>This application has been blocked by your security policy. Click below to request access.</string>
			<key>EventDetailURL</key>
			<string>%s/proposals?hash=%%file_identifier%%&amp;machine=%%machine_id%%</string>
			<key>EventDetailText</key>
			<string>Request Access</string>%s
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>Configures NorthPole Security Santa endpoint security for %s</string>
	<key>PayloadDisplayName</key>
	<string>Santa Configuration - %s</string>
	<key>PayloadIdentifier</key>
	<string>com.northpolesec.santa.config.%s</string>
	<key>PayloadOrganization</key>
	<string>%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadScope</key>
	<string>System</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`

	return fmt.Sprintf(mobileConfigTemplate,
		payloadUUID, organizationName, payloadUUID,
		syncBaseURL, mode, machineID, machineOwner, uploadInterval, syncBaseURL,
		syncExtraHeaders,
		machineID, machineID, profileUUID, organizationName, profileUUID)
}
