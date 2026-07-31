use std::sync::Arc;

use crate::error::AppError;
use crate::id::MediaType;
use crate::services::api_key_pool::ApiKeyPool;
use crate::services::retry::{self, MDBLIST_RETRY};
use serde::Deserialize;

#[derive(Clone)]
pub struct MdblistClient {
    key_pool: Arc<ApiKeyPool>,
    http: reqwest::Client,
}

#[derive(Debug, Deserialize)]
pub struct MdblistResponse {
    #[serde(default)]
    pub ratings: Vec<MdblistRating>,
    #[serde(default)]
    pub ids: MdblistIds,
    #[serde(default)]
    pub score: Option<f64>,
}

#[derive(Debug, Default, Deserialize)]
pub struct MdblistIds {
    pub imdb: Option<String>,
    pub tmdb: Option<u64>,
    pub tvdb: Option<u64>,
}

#[derive(Debug, Deserialize)]
pub struct MdblistRating {
    pub source: String,
    pub value: Option<f64>,
    pub score: Option<f64>,
    pub votes: Option<i64>,
}

fn mdblist_kind(media_type: &MediaType) -> Result<&'static str, AppError> {
    match media_type {
        MediaType::Movie => Ok("movie"),
        MediaType::Tv => Ok("show"),
        MediaType::Episode => Err(AppError::Other("mdblist does not support episode ratings".into())),
    }
}

fn imdb_ratings_url(kind: &str, imdb_id: &str) -> String {
    format!("https://api.mdblist.com/imdb/{kind}/{imdb_id}")
}

fn tmdb_ratings_url(kind: &str, tmdb_id: u64) -> String {
    format!("https://api.mdblist.com/tmdb/{kind}/{tmdb_id}")
}

impl MdblistClient {
    pub fn new(key_pool: ApiKeyPool, http: reqwest::Client) -> Self {
        Self { key_pool: Arc::new(key_pool), http }
    }

    pub fn active_key_hash(&self) -> String {
        self.key_pool.active_key_hash()
    }

    async fn fetch(&self, url: &str) -> Result<MdblistResponse, AppError> {
        let api_key = self.key_pool.active_key_raw();
        let key_hash = self.key_pool.active_key_hash();

        let url = url.to_owned();
        let resp = retry::send_with_retry(&MDBLIST_RETRY, || {
            self.http
                .get(&url)
                .query(&[("apikey", api_key.as_str())])
                .send()
        })
        .await?;

        let was_429 = resp.status() == reqwest::StatusCode::TOO_MANY_REQUESTS;

        let result = resp.error_for_status();

        if was_429 {
            self.key_pool.report_429();
            tracing::warn!(
                key = %key_hash,
                "mdblist API key rate limited (429)"
            );
        } else {
            self.key_pool.report_success();
        }

        let resp = result?;
        Ok(resp.json().await?)
    }

    pub async fn get_ratings(
        &self,
        imdb_id: &str,
        media_type: &MediaType,
    ) -> Result<MdblistResponse, AppError> {
        let kind = mdblist_kind(media_type)?;
        self.fetch(&imdb_ratings_url(kind, imdb_id)).await
    }

    pub async fn get_ratings_by_tmdb(
        &self,
        tmdb_id: u64,
        media_type: &MediaType,
    ) -> Result<MdblistResponse, AppError> {
        let kind = mdblist_kind(media_type)?;
        self.fetch(&tmdb_ratings_url(kind, tmdb_id)).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mdblist_kind_maps_movie_and_tv() {
        assert_eq!(mdblist_kind(&MediaType::Movie).unwrap(), "movie");
        assert_eq!(mdblist_kind(&MediaType::Tv).unwrap(), "show");
    }

    #[test]
    fn mdblist_kind_rejects_episode() {
        assert!(mdblist_kind(&MediaType::Episode).is_err());
    }

    #[test]
    fn imdb_ratings_url_format() {
        assert_eq!(imdb_ratings_url("show", "tt2560140"), "https://api.mdblist.com/imdb/show/tt2560140");
        assert_eq!(imdb_ratings_url("movie", "tt0111161"), "https://api.mdblist.com/imdb/movie/tt0111161");
    }

    #[test]
    fn tmdb_ratings_url_format() {
        assert_eq!(tmdb_ratings_url("show", 1429), "https://api.mdblist.com/tmdb/show/1429");
        assert_eq!(tmdb_ratings_url("movie", 550), "https://api.mdblist.com/tmdb/movie/550");
    }
}
