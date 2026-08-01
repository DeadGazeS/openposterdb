use std::sync::Arc;

use crate::error::AppError;
use crate::services::api_key_pool::ApiKeyPool;
use crate::services::retry::{self, OMDB_RETRY};
use serde::Deserialize;

#[derive(Clone)]
pub struct OmdbClient {
    key_pool: Arc<ApiKeyPool>,
    http: reqwest::Client,
}

#[derive(Debug, Deserialize)]
pub struct OmdbResponse {
    #[serde(rename = "Ratings", default)]
    pub ratings: Vec<OmdbRating>,
    #[serde(rename = "imdbRating")]
    pub imdb_rating: Option<String>,
    #[serde(rename = "Metascore")]
    pub metascore: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct OmdbRating {
    #[serde(rename = "Source")]
    pub source: String,
    #[serde(rename = "Value")]
    pub value: String,
}

impl OmdbClient {
    pub fn new(key_pool: ApiKeyPool, http: reqwest::Client) -> Self {
        Self { key_pool: Arc::new(key_pool), http }
    }

    pub async fn get_ratings(&self, imdb_id: &str) -> Result<OmdbResponse, AppError> {
        let api_key = self.key_pool.active_key_raw();
        let imdb_id = imdb_id.to_owned();
        let resp = retry::send_with_retry(&OMDB_RETRY, || {
            self.http
                .get("https://www.omdbapi.com/")
                .query(&[("apikey", api_key.as_str()), ("i", imdb_id.as_str())])
                .send()
        })
        .await?
        .error_for_status()?;
        Ok(resp.json().await?)
    }
}
