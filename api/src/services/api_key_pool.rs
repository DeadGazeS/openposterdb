use std::sync::Mutex;
use std::time::{Duration, Instant};

use sha2::{Digest, Sha256};
use zeroize::Zeroizing;

const ROTATION_COOLDOWN: Duration = Duration::from_secs(86400);
const MAX_CONSECUTIVE_429S: u32 = 3;

fn hash_key(key: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(key.as_bytes());
    format!("{:x}", hasher.finalize())
        .chars()
        .take(12)
        .collect()
}

struct ApiKey {
    raw: Zeroizing<String>,
    hashed: String,
}

#[derive(Debug)]
struct PoolState {
    active_index: usize,
    consecutive_429s: u32,
    last_rotation: Option<Instant>,
}

#[derive(Clone)]
pub struct ApiKeyPool {
    keys: Vec<ApiKey>,
    state: std::sync::Arc<Mutex<PoolState>>,
}

impl ApiKeyPool {
    pub fn new(keys: Vec<String>) -> Self {
        assert!(!keys.is_empty(), "ApiKeyPool requires at least one key");
        let keys: Vec<ApiKey> = keys
            .into_iter()
            .map(|k| {
                let hashed = hash_key(&k);
                ApiKey {
                    raw: Zeroizing::new(k),
                    hashed,
                }
            })
            .collect();
        Self {
            keys,
            state: std::sync::Arc::new(Mutex::new(PoolState {
                active_index: 0,
                consecutive_429s: 0,
                last_rotation: None,
            })),
        }
    }

    pub fn len(&self) -> usize {
        self.keys.len()
    }

    pub fn active_key_raw(&self) -> Zeroizing<String> {
        let mut guard = self.state.lock().unwrap();
        self.maybe_reset_cooldown(&mut guard);
        self.keys[guard.active_index].raw.clone()
    }

    pub fn active_key_hash(&self) -> String {
        let guard = self.state.lock().unwrap();
        self.keys[guard.active_index].hashed.clone()
    }

    pub fn report_429(&self) {
        let mut guard = self.state.lock().unwrap();

        if self.keys.len() <= 1 {
            return;
        }

        self.maybe_reset_cooldown(&mut guard);

        guard.consecutive_429s += 1;
        if guard.consecutive_429s >= MAX_CONSECUTIVE_429S {
            self.rotate(&mut guard);
        }
    }

    pub fn report_success(&self) {
        let mut guard = self.state.lock().unwrap();
        guard.consecutive_429s = 0;

        if self.keys.len() > 1 {
            self.maybe_reset_cooldown(&mut guard);
        }
    }

    fn maybe_reset_cooldown(&self, guard: &mut PoolState) {
        if let Some(rotated_at) = guard.last_rotation {
            if rotated_at.elapsed() >= ROTATION_COOLDOWN {
                let old_hash = self.keys[guard.active_index].hashed.clone();
                guard.active_index = 0;
                guard.last_rotation = None;
                guard.consecutive_429s = 0;
                let new_hash = self.keys[0].hashed.clone();
                tracing::info!(
                    old_key = %old_hash,
                    new_key = %new_hash,
                    "API key cooldown expired, returning to primary key"
                );
            }
        }
    }

    fn rotate(&self, guard: &mut PoolState) {
        let old_index = guard.active_index;
        let old_hash = self.keys[old_index].hashed.clone();
        guard.active_index = (guard.active_index + 1) % self.keys.len();
        guard.last_rotation = Some(Instant::now());
        guard.consecutive_429s = 0;
        let new_hash = self.keys[guard.active_index].hashed.clone();
        tracing::warn!(
            old_key = %old_hash,
            new_key = %new_hash,
            consecutive_429s = MAX_CONSECUTIVE_429S,
            "rotating API key after consecutive rate limits"
        );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn single_key_never_rotates() {
        let pool = ApiKeyPool::new(vec!["key1".into()]);
        assert_eq!(pool.len(), 1);
        let k1 = pool.active_key_raw();
        pool.report_429();
        pool.report_429();
        pool.report_429();
        pool.report_429();
        let k2 = pool.active_key_raw();
        assert_eq!(*k1, *k2); // same key, no rotation possible
    }

    #[test]
    fn rotation_after_consecutive_429s() {
        let pool = ApiKeyPool::new(vec!["key1".into(), "key2".into()]);
        let k1 = pool.active_key_raw();
        assert_eq!(*k1, "key1");

        // Two 429s should not trigger rotation
        pool.report_429();
        pool.report_429();
        let k2 = pool.active_key_raw();
        assert_eq!(*k2, "key1");

        // Third 429 triggers rotation
        pool.report_429();
        let k3 = pool.active_key_raw();
        assert_eq!(*k3, "key2");
    }

    #[test]
    fn success_resets_consecutive_count() {
        let pool = ApiKeyPool::new(vec!["key1".into(), "key2".into()]);

        pool.report_429();
        pool.report_429();
        pool.report_success(); // resets counter
        pool.report_429();
        pool.report_429();
        // Still only 2 consecutive, no rotation
        let k = pool.active_key_raw();
        assert_eq!(*k, "key1");
    }

    #[test]
    fn wraps_around_to_first_key() {
        let pool = ApiKeyPool::new(vec!["key1".into(), "key2".into(), "key3".into()]);

        // Rotate twice: key1 -> key2 -> key3 -> key1
        pool.report_429();
        pool.report_429();
        pool.report_429();
        assert_eq!(*pool.active_key_raw(), "key2");

        pool.report_429();
        pool.report_429();
        pool.report_429();
        assert_eq!(*pool.active_key_raw(), "key3");

        pool.report_429();
        pool.report_429();
        pool.report_429();
        assert_eq!(*pool.active_key_raw(), "key1");
    }

    #[test]
    fn hash_is_deterministic() {
        let h1 = hash_key("testkey123");
        let h2 = hash_key("testkey123");
        assert_eq!(h1, h2);
    }

    #[test]
    fn hash_is_different_for_different_keys() {
        let h1 = hash_key("key_a");
        let h2 = hash_key("key_b");
        assert_ne!(h1, h2);
    }

    #[test]
    fn hash_length_is_12() {
        assert_eq!(hash_key("some_key").len(), 12);
    }
}
