//! Nest Rust SDK - async client for the Nest storage platform.
//!
//! # Example
//! ```no_run
//! use nest::{NestClient, DataResourceSpec};
//!
//! #[tokio::main]
//! async fn main() {
//!     let client = NestClient::new("https://nest.acme.com", "sk-...");
//!     let resources = client.data_resources().list("acme").await.unwrap();
//!     println!("{:?}", resources);
//! }
//! ```

use serde::{Deserialize, Serialize};

/// DataResourceSpec for creating a new resource.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DataResourceSpec {
    pub name: String,
    #[serde(rename = "type")]
    pub resource_type: String,
    pub class: String,
    pub tenant: String,
}

/// DataResource as returned by the API.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DataResource {
    pub id: String,
    pub name: String,
    #[serde(rename = "type")]
    pub resource_type: String,
    pub class: String,
    pub tenant: String,
    pub status: String,
    pub endpoint: Option<String>,
}

/// NestClient is the main entry point.
pub struct NestClient {
    base_url: String,
    token: String,
}

impl NestClient {
    /// Create a new client.
    pub fn new(base_url: impl Into<String>, token: impl Into<String>) -> Self {
        Self {
            base_url: base_url.into().trim_end_matches('/').to_owned(),
            token: token.into(),
        }
    }

    /// Get the data resources sub-client.
    pub fn data_resources(&self) -> DataResourcesClient<'_> {
        DataResourcesClient { client: self }
    }
}

/// DataResourcesClient manages data resource operations.
pub struct DataResourcesClient<'a> {
    client: &'a NestClient,
}

impl<'a> DataResourcesClient<'a> {
    /// List all data resources for a tenant.
    /// NOTE: Stub — production should use reqwest or hyper.
    pub async fn list(&self, tenant: &str) -> Result<Vec<DataResource>, String> {
        let _url = format!(
            "{}/api/v1/tenants/{}/dataresources",
            self.client.base_url, tenant
        );
        let _auth = format!("Bearer {}", self.client.token);
        // TODO: make actual HTTP request using reqwest
        Ok(vec![])
    }

    /// Create a new data resource.
    pub async fn create(
        &self,
        tenant: &str,
        spec: DataResourceSpec,
    ) -> Result<String, String> {
        let _url = format!(
            "{}/api/v1/tenants/{}/dataresources",
            self.client.base_url, tenant
        );
        let _ = serde_json::to_string(&spec).map_err(|e| e.to_string())?;
        // TODO: make actual HTTP request using reqwest
        Ok(String::new())
    }

    /// Get a single data resource.
    pub async fn get(&self, tenant: &str, name: &str) -> Result<DataResource, String> {
        let _url = format!(
            "{}/api/v1/tenants/{}/dataresources/{}",
            self.client.base_url, tenant, name
        );
        let _auth = format!("Bearer {}", self.client.token);
        // TODO: make actual HTTP request using reqwest
        Err("not implemented".to_string())
    }

    /// Delete a data resource.
    pub async fn delete(&self, tenant: &str, name: &str) -> Result<(), String> {
        let _url = format!(
            "{}/api/v1/tenants/{}/dataresources/{}",
            self.client.base_url, tenant, name
        );
        let _auth = format!("Bearer {}", self.client.token);
        // TODO: make actual HTTP request using reqwest
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_client_creation() {
        let client = NestClient::new("https://nest.acme.com", "sk-test");
        assert_eq!(client.base_url, "https://nest.acme.com");
        assert_eq!(client.token, "sk-test");
    }

    #[test]
    fn test_client_strips_trailing_slash() {
        let client = NestClient::new("https://nest.acme.com/", "sk-test");
        assert_eq!(client.base_url, "https://nest.acme.com");
    }

    #[test]
    fn test_spec_serialization() {
        let spec = DataResourceSpec {
            name: "my-db".to_string(),
            resource_type: "postgres".to_string(),
            class: "standard".to_string(),
            tenant: "acme".to_string(),
        };
        let json = serde_json::to_string(&spec).unwrap();
        assert!(json.contains("\"name\":\"my-db\""));
        assert!(json.contains("\"type\":\"postgres\""));
    }
}
