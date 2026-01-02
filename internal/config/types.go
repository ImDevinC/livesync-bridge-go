package config

// Config represents the overall configuration for LiveSync Bridge
type Config struct {
	Peers []PeerConf `json:"peers"`
}

// PeerConf is the interface for all peer configurations
type PeerConf interface {
	GetType() string
	GetName() string
	GetGroup() string
	GetBaseDir() string
}

// PeerStorageConf represents configuration for a filesystem storage peer
type PeerStorageConf struct {
	Type               string `json:"type"`
	Name               string `json:"name"`
	Group              string `json:"group,omitempty"`
	BaseDir            string `json:"baseDir"`
	ScanOfflineChanges *bool  `json:"scanOfflineChanges,omitempty"`
	// Processor field deferred for later implementation
	// UseChokidar field not needed (using fsnotify)
}

func (p PeerStorageConf) GetType() string    { return p.Type }
func (p PeerStorageConf) GetName() string    { return p.Name }
func (p PeerStorageConf) GetGroup() string   { return p.Group }
func (p PeerStorageConf) GetBaseDir() string { return p.BaseDir }

// PeerCouchDBConf represents configuration for a CouchDB peer
type PeerCouchDBConf struct {
	Type                string `json:"type"`
	Name                string `json:"name"`
	Group               string `json:"group,omitempty"`
	Database            string `json:"database"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	URL                 string `json:"url"`
	CustomChunkSize     *int   `json:"customChunkSize,omitempty"`
	MinimumChunkSize    *int   `json:"minimumChunkSize,omitempty"`
	Passphrase          string `json:"passphrase"`
	ObfuscatePassphrase string `json:"obfuscatePassphrase,omitempty"` // Deferred feature
	BaseDir             string `json:"baseDir"`
	UseRemoteTweaks     *bool  `json:"useRemoteTweaks,omitempty"` // Deferred feature
	EnableCompression   *bool  `json:"enableCompression,omitempty"`
	InitialSync         *bool  `json:"initialSync,omitempty"` // Sync existing documents on first run
}

func (p PeerCouchDBConf) GetType() string    { return p.Type }
func (p PeerCouchDBConf) GetName() string    { return p.Name }
func (p PeerCouchDBConf) GetGroup() string   { return p.Group }
func (p PeerCouchDBConf) GetBaseDir() string { return p.BaseDir }

// IsCouchDBPeer checks if a peer is a CouchDB peer
func IsCouchDBPeer(peer PeerConf) bool {
	return peer.GetType() == "couchdb"
}

// IsStoragePeer checks if a peer is a storage peer
func IsStoragePeer(peer PeerConf) bool {
	return peer.GetType() == "storage"
}
