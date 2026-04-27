package io.penguintech.nest;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.concurrent.CompletableFuture;

/**
 * Nest Java SDK client for the Nest storage platform.
 *
 * <p>Usage:
 * <pre>{@code
 * NestClient client = new NestClient.Builder()
 *     .baseUrl("https://nest.acme.com")
 *     .token("sk-...")
 *     .build();
 * }</pre>
 */
public class NestClient {
    private final String baseUrl;
    private final String token;
    private final HttpClient httpClient;

    private NestClient(Builder builder) {
        this.baseUrl = builder.baseUrl.replaceAll("/$", "");
        this.token = builder.token;
        this.httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(10))
            .build();
    }

    /** Send an authenticated HTTP request. */
    public CompletableFuture<String> request(String method, String path, String body) {
        HttpRequest.Builder req = HttpRequest.newBuilder()
            .uri(URI.create(baseUrl + path))
            .header("Authorization", "Bearer " + token)
            .header("Content-Type", "application/json")
            .timeout(Duration.ofSeconds(30));

        if (body != null && !body.isEmpty()) {
            req.method(method, HttpRequest.BodyPublishers.ofString(body));
        } else if ("GET".equals(method) || "DELETE".equals(method)) {
            req.method(method, HttpRequest.BodyPublishers.noBody());
        } else {
            req.method(method, HttpRequest.BodyPublishers.noBody());
        }

        return httpClient.sendAsync(req.build(), HttpResponse.BodyHandlers.ofString())
            .thenApply(HttpResponse::body);
    }

    /**
     * Get the base URL for this client.
     */
    public String getBaseUrl() {
        return baseUrl;
    }

    /**
     * Get the token for this client.
     */
    public String getToken() {
        return token;
    }

    /** Builder for NestClient. */
    public static class Builder {
        private String baseUrl = "";
        private String token = "";

        /**
         * Set the base URL for the Nest API.
         */
        public Builder baseUrl(String baseUrl) {
            this.baseUrl = baseUrl;
            return this;
        }

        /**
         * Set the authentication token.
         */
        public Builder token(String token) {
            this.token = token;
            return this;
        }

        /**
         * Build the NestClient.
         */
        public NestClient build() {
            if (baseUrl.isEmpty()) {
                throw new IllegalArgumentException("baseUrl is required");
            }
            if (token.isEmpty()) {
                throw new IllegalArgumentException("token is required");
            }
            return new NestClient(this);
        }
    }
}
