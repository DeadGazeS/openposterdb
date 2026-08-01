use std::sync::RwLock;

use aes_gcm::aead::{Aead, KeyInit, OsRng};
use aes_gcm::{AeadCore, Aes256Gcm, Nonce};
use base64::Engine;
use hkdf::Hkdf;
use sea_orm::DatabaseConnection;
use sha2::Sha256;
use zeroize::Zeroizing;

use crate::error::AppError;
use crate::services::db;

fn derive_cipher_key(jwt_secret: &[u8]) -> [u8; 32] {
    let hkdf = Hkdf::<Sha256>::new(None, jwt_secret);
    let mut key = [0u8; 32];
    hkdf.expand(b"openposterdb-service-keys", &mut key)
        .expect("HKDF expand should not fail for 32 bytes");
    key
}

fn encrypt(value: &str, cipher: &Aes256Gcm) -> String {
    let nonce = Aes256Gcm::generate_nonce(&mut OsRng);
    let ciphertext = cipher
        .encrypt(&nonce, value.as_bytes())
        .expect("encryption should not fail");
    let mut combined = nonce.to_vec();
    combined.extend_from_slice(&ciphertext);
    base64::engine::general_purpose::STANDARD.encode(&combined)
}

fn decrypt(encrypted: &str, cipher: &Aes256Gcm) -> Result<Zeroizing<String>, AppError> {
    let data = base64::engine::general_purpose::STANDARD
        .decode(encrypted)
        .map_err(|e| AppError::Other(format!("base64 decode: {e}")))?;
    if data.len() < 12 {
        return Err(AppError::Other("encrypted data too short".into()));
    }
    let (nonce_bytes, ciphertext) = data.split_at(12);
    let nonce = Nonce::from_slice(nonce_bytes);
    let plaintext = cipher
        .decrypt(nonce, ciphertext)
        .map_err(|_| AppError::Other("decryption failed — JWT_SECRET may have changed".into()))?;
    String::from_utf8(plaintext)
        .map(Zeroizing::new)
        .map_err(|_| AppError::Other("decrypted data is not valid UTF-8".into()))
}

pub fn mask_key(key: &str) -> String {
    let n = (key.len() / 4).min(4);
    if n == 0 {
        return "****".to_string();
    }
    let prefix: String = key.chars().take(n).collect();
    let suffix: String = key.chars().rev().take(n).collect::<String>()
        .chars().rev().collect();
    format!("{prefix}...{suffix}")
}

#[derive(Debug, Clone, Default)]
struct KeyCache {
    mdblist: Vec<String>,
    omdb: Vec<String>,
    fanart: Vec<String>,
    trakt: Vec<String>,
}

#[derive(Clone)]
pub struct ServiceKeyManager {
    db: DatabaseConnection,
    cipher: std::sync::Arc<Aes256Gcm>,
    http: reqwest::Client,
    keys: std::sync::Arc<RwLock<KeyCache>>,
    // Which services are locked by env vars
    env_mdblist: bool,
    env_omdb: bool,
    env_fanart: bool,
    env_trakt: bool,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ServiceKeyStatus {
    pub locked: bool,
    pub has_key: bool,
    pub masked: Option<String>,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ServiceKeysResponse {
    pub mdblist: ServiceKeyStatus,
    pub omdb: ServiceKeyStatus,
    pub fanart: ServiceKeyStatus,
    pub trakt: ServiceKeyStatus,
}

#[derive(Debug, Clone, serde::Deserialize)]
pub struct ServiceKeysUpdate {
    #[serde(default)]
    pub mdblist: Option<String>,
    #[serde(default)]
    pub omdb: Option<String>,
    #[serde(default)]
    pub fanart: Option<String>,
    #[serde(default)]
    pub trakt: Option<String>,
}

impl ServiceKeyManager {
    pub fn new(
        db: DatabaseConnection,
        jwt_secret: &[u8],
        http: reqwest::Client,
        env_mdblist: Option<Vec<String>>,
        env_omdb: Option<String>,
        env_fanart: Option<String>,
        env_trakt: Option<String>,
    ) -> Self {
        let cipher_key = derive_cipher_key(jwt_secret);
        let cipher = Aes256Gcm::new_from_slice(&cipher_key)
            .expect("AES-256-GCM key must be 32 bytes");

        let mut cache = KeyCache::default();

        let env_mdblist_locked = env_mdblist.is_some();
        if let Some(keys) = env_mdblist {
            cache.mdblist = keys;
        }

        let env_omdb_locked = env_omdb.is_some();
        if let Some(k) = env_omdb {
            cache.omdb = vec![k];
        }

        let env_fanart_locked = env_fanart.is_some();
        if let Some(k) = env_fanart {
            cache.fanart = vec![k];
        }

        let env_trakt_locked = env_trakt.is_some();
        if let Some(k) = env_trakt {
            cache.trakt = vec![k];
        }

        Self {
            db,
            cipher: std::sync::Arc::new(cipher),
            http,
            keys: std::sync::Arc::new(RwLock::new(cache)),
            env_mdblist: env_mdblist_locked,
            env_omdb: env_omdb_locked,
            env_fanart: env_fanart_locked,
            env_trakt: env_trakt_locked,
        }
    }

    /// Load keys from DB for services that don't have env-provided keys.
    pub async fn init(&self) {
        if !self.env_mdblist {
            if let Some(v) = self.load_from_db("mdblist").await {
                let keys: Vec<String> = v.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                if !keys.is_empty() {
                    self.keys.write().unwrap().mdblist = keys;
                }
            }
        }
        if !self.env_omdb {
            if let Some(v) = self.load_from_db("omdb").await {
                let keys: Vec<String> = v.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                if !keys.is_empty() {
                    self.keys.write().unwrap().omdb = keys;
                }
            }
        }
        if !self.env_fanart {
            if let Some(v) = self.load_from_db("fanart").await {
                let keys: Vec<String> = v.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                if !keys.is_empty() {
                    self.keys.write().unwrap().fanart = keys;
                }
            }
        }
        if !self.env_trakt {
            if let Some(v) = self.load_from_db("trakt").await {
                let keys: Vec<String> = v.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                if !keys.is_empty() {
                    self.keys.write().unwrap().trakt = keys;
                }
            }
        }
    }

    async fn load_from_db(&self, service: &str) -> Option<String> {
        let encrypted = db::get_global_setting(&self.db, &format!("service_key_{service}"))
            .await
            .ok()
            .flatten()?;
        decrypt(&encrypted, &self.cipher).ok().map(|z| z.to_string())
    }

    pub fn mdblist_keys(&self) -> Vec<String> {
        self.keys.read().unwrap().mdblist.clone()
    }

    pub fn omdb_keys(&self) -> Vec<String> {
        self.keys.read().unwrap().omdb.clone()
    }

    pub fn fanart_keys(&self) -> Vec<String> {
        self.keys.read().unwrap().fanart.clone()
    }

    pub fn trakt_client_ids(&self) -> Vec<String> {
        self.keys.read().unwrap().trakt.clone()
    }

    pub fn env_locked(&self, service: &str) -> bool {
        match service {
            "mdblist" => self.env_mdblist,
            "omdb" => self.env_omdb,
            "fanart" => self.env_fanart,
            "trakt" => self.env_trakt,
            _ => false,
        }
    }

    pub fn http_client(&self) -> &reqwest::Client {
        &self.http
    }

    pub fn get_status(&self) -> ServiceKeysResponse {
        let keys = self.keys.read().unwrap();
        ServiceKeysResponse {
            mdblist: ServiceKeyStatus {
                locked: self.env_mdblist,
                has_key: !keys.mdblist.is_empty(),
                masked: if keys.mdblist.is_empty() { None }
                    else { Some(keys.mdblist.iter().map(|k| mask_key(k)).collect::<Vec<_>>().join(", ")) },
            },
            omdb: ServiceKeyStatus {
                locked: self.env_omdb,
                has_key: !keys.omdb.is_empty(),
                masked: if keys.omdb.is_empty() { None }
                    else { Some(keys.omdb.iter().map(|k| mask_key(k)).collect::<Vec<_>>().join(", ")) },
            },
            fanart: ServiceKeyStatus {
                locked: self.env_fanart,
                has_key: !keys.fanart.is_empty(),
                masked: if keys.fanart.is_empty() { None }
                    else { Some(keys.fanart.iter().map(|k| mask_key(k)).collect::<Vec<_>>().join(", ")) },
            },
            trakt: ServiceKeyStatus {
                locked: self.env_trakt,
                has_key: !keys.trakt.is_empty(),
                masked: if keys.trakt.is_empty() { None }
                    else { Some(keys.trakt.iter().map(|k| mask_key(k)).collect::<Vec<_>>().join(", ")) },
            },
        }
    }

    pub async fn update_keys(&self, update: &ServiceKeysUpdate) -> Result<(), AppError> {
        if !self.env_mdblist {
            if let Some(ref keys) = update.mdblist {
                self.store_key("mdblist", keys).await?;
                let parsed: Vec<String> = keys.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                self.keys.write().unwrap().mdblist = parsed;
            }
        }
        if !self.env_omdb {
            if let Some(ref keys) = update.omdb {
                self.store_key("omdb", keys).await?;
                let parsed: Vec<String> = keys.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                self.keys.write().unwrap().omdb = parsed;
            }
        }
        if !self.env_fanart {
            if let Some(ref keys) = update.fanart {
                self.store_key("fanart", keys).await?;
                let parsed: Vec<String> = keys.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                self.keys.write().unwrap().fanart = parsed;
            }
        }
        if !self.env_trakt {
            if let Some(ref keys) = update.trakt {
                self.store_key("trakt", keys).await?;
                let parsed: Vec<String> = keys.split(',').map(|k| k.trim().to_string()).filter(|k| !k.is_empty()).collect();
                self.keys.write().unwrap().trakt = parsed;
            }
        }
        Ok(())
    }

    async fn store_key(&self, service: &str, value: &str) -> Result<(), AppError> {
        let encrypted = encrypt(value, &self.cipher);
        db::set_global_setting(
            &self.db,
            &format!("service_key_{service}"),
            &encrypted,
        )
        .await
    }
}
