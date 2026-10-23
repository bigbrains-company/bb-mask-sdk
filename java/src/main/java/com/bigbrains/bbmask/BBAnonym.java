package com.bigbrains.bbmask;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.UncheckedIOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.UUID;

/** Tek anahtarın istemcisi. init gerekmez. */
public final class BBAnonym {
    private static final String NOT_OPEN = "Bu uç henüz açık değil.";
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final TypeReference<Map<String, Object>> MAP = new TypeReference<>() {
    };

    private final String baseUrl;
    private final String token;
    private final Duration timeout;
    private final HttpClient http;

    public BBAnonym(String baseUrl, String token) {
        this(baseUrl, token, null);
    }

    public BBAnonym(String baseUrl, String token, Duration timeout) {
        String trimmed = baseUrl == null ? "" : baseUrl.trim();
        while (trimmed.endsWith("/")) {
            trimmed = trimmed.substring(0, trimmed.length() - 1);
        }
        this.baseUrl = trimmed.isEmpty() ? BbMask.DEFAULT_BASE_URL : trimmed;
        this.token = token == null ? "" : token;
        this.timeout = timeout == null || timeout.isZero() || timeout.isNegative()
                ? BbMask.DEFAULT_TIMEOUT
                : timeout;
        this.http = HttpClient.newBuilder()
                .version(HttpClient.Version.HTTP_1_1)
                .connectTimeout(this.timeout)
                .build();
    }

    public String getBaseUrl() {
        return baseUrl;
    }

    public Duration getTimeout() {
        return timeout;
    }

    public Tagged anonymize(String text) {
        return anonymize(text, null);
    }

    public Tagged anonymize(String text, Options options) {
        Options resolved = Options.resolve(options);
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("text", text);
        payload.put("persist", resolved.persist);
        payload.put("placeholder_style", resolved.placeholderStyle);
        payload.put("keep_legal_form", resolved.keepLegalForm);
        payload.put("health_data", resolved.healthData);
        payload.put("llm_review", resolved.llmReview);
        try {
            Exchange exchange = send("POST", "/v1/anonymize", null, JSON.writeValueAsBytes(payload), "application/json");
            return toTagged(exchange.body, resolved.persist, exchange.requestId);
        } catch (JsonProcessingException error) {
            throw new IllegalStateException(error);
        }
    }

    public Tagged anonymizeFile(String filename, byte[] content) {
        return anonymizeFile(filename, content, null);
    }

    public Tagged anonymizeFile(String filename, byte[] content, Options options) {
        Options resolved = Options.resolve(options);
        return sendForm("/v1/anonymize/file", resolved, List.of(new FilePart("file", filename, content)));
    }

    public Tagged anonymizePath(String path) {
        return anonymizePath(path, null);
    }

    public Tagged anonymizePath(String path, Options options) {
        try {
            byte[] content = Files.readAllBytes(Path.of(path));
            return anonymizeFile(Path.of(path).getFileName().toString(), content, options);
        } catch (BBError error) {
            throw error;
        } catch (IOException error) {
            throw new UncheckedIOException(error);
        }
    }

    public Tagged anonymizeDocuments(List<String> paths) {
        return anonymizeDocuments(paths, null);
    }

    public Tagged anonymizeDocuments(List<String> paths, Options options) {
        Options resolved = Options.resolve(options);
        if (paths == null || paths.size() < 1 || paths.size() > 3) {
            throw new BBError(400, "Bir istekte 1–3 dosya", null);
        }
        List<FilePart> files = new ArrayList<>();
        try {
            for (String path : paths) {
                files.add(new FilePart("files", Path.of(path).getFileName().toString(), Files.readAllBytes(Path.of(path))));
            }
        } catch (IOException error) {
            throw new UncheckedIOException(error);
        }
        return sendForm("/v1/anonymize/documents", resolved, files);
    }

    public String restore(Object docIdOrState, String modelText) {
        Map<String, Object> payload = new LinkedHashMap<>();
        payload.put("text", modelText);
        if (docIdOrState instanceof Map<?, ?> map) {
            payload.put("state", map);
        } else if (docIdOrState instanceof String id) {
            payload.put("doc_id", id);
        } else {
            throw new BBError(400, "restore doc_id (string) veya state (map) ister", null);
        }
        Map<String, Object> body = postJson("/v1/restore", payload);
        Object text = body.get("text");
        return text instanceof String value ? value : "";
    }

    public Map<String, Object> getMap(String docId, boolean once) {
        Exchange exchange = send("GET", "/v1/mappings/" + pathEscape(docId), "once=" + once, null, null);
        Object state = exchange.body.get("state");
        if (state instanceof Map<?, ?> map) {
            return cast(map);
        }
        return new LinkedHashMap<>();
    }

    public boolean delete(String docId) {
        Exchange exchange = send("DELETE", "/v1/mappings/" + pathEscape(docId), null, null, null);
        return Boolean.TRUE.equals(exchange.body.get("deleted"));
    }

    public String complete(String text) {
        throw new BBError(501, NOT_OPEN, null);
    }

    public String completeFile(String path) {
        throw new BBError(501, NOT_OPEN, null);
    }

    public String completeDocuments(List<String> paths) {
        throw new BBError(501, NOT_OPEN, null);
    }

    private Tagged sendForm(String path, Options options, List<FilePart> files) {
        String boundary = "bbmask" + UUID.randomUUID().toString().replace("-", "");
        Exchange exchange = send("POST", path, null, multipart(options, files, boundary), "multipart/form-data; boundary=" + boundary);
        return toTagged(exchange.body, options.persist, exchange.requestId);
    }

    private Map<String, Object> postJson(String path, Map<String, Object> payload) {
        try {
            return send("POST", path, null, JSON.writeValueAsBytes(payload), "application/json").body;
        } catch (JsonProcessingException error) {
            throw new IllegalStateException(error);
        }
    }

    private Exchange send(String method, String path, String query, byte[] body, String contentType) {
        String url = baseUrl + path + (query == null || query.isEmpty() ? "" : "?" + query);
        HttpRequest.Builder builder;
        try {
            builder = HttpRequest.newBuilder(URI.create(url)).timeout(timeout).header("Authorization", "Bearer " + token);
        } catch (IllegalArgumentException error) {
            throw new BBError(503, unreachable(), null);
        }
        if ("POST".equals(method)) {
            builder.header("Content-Type", contentType);
            builder.POST(HttpRequest.BodyPublishers.ofByteArray(body == null ? new byte[0] : body));
        } else if ("DELETE".equals(method)) {
            builder.DELETE();
        } else {
            builder.GET();
        }
        try {
            HttpResponse<byte[]> response = http.send(builder.build(), HttpResponse.BodyHandlers.ofByteArray());
            String requestId = response.headers().firstValue("x-request-id").orElse(null);
            return new Exchange(parse(response.statusCode(), requestId, response.body()), requestId);
        } catch (HttpTimeoutException error) {
            throw new BBError(504, "Zaman aşımı (" + formatSeconds(timeout) + " sn). Ücret düşmüş olabilir.", null);
        } catch (IOException error) {
            throw new BBError(503, unreachable(), null);
        } catch (InterruptedException error) {
            Thread.currentThread().interrupt();
            throw new BBError(503, unreachable(), null);
        }
    }

    private String unreachable() {
        return "Servise ulaşılamadı (" + baseUrl + ")";
    }

    private static String formatSeconds(Duration duration) {
        String text = String.format(Locale.US, "%.3f", duration.toNanos() / 1_000_000_000d);
        while (text.contains(".") && (text.endsWith("0") || text.endsWith("."))) {
            text = text.substring(0, text.length() - 1);
        }
        if (text.isEmpty() || "-".equals(text)) {
            return "0";
        }
        return text;
    }

    private Map<String, Object> parse(int status, String requestId, byte[] raw) {
        if (status >= 400) {
            throw new BBError(status, errorDetail(raw), requestId);
        }
        try {
            Map<String, Object> body = JSON.readValue(raw, MAP);
            if (body == null) {
                throw new BBError(502, "Servis JSON dönmedi", requestId);
            }
            return body;
        } catch (BBError error) {
            throw error;
        } catch (IOException error) {
            throw new BBError(502, "Servis JSON dönmedi", requestId);
        }
    }

    private static String errorDetail(byte[] raw) {
        String text = new String(raw, StandardCharsets.UTF_8);
        try {
            Object parsed = JSON.readValue(raw, Object.class);
            if (!(parsed instanceof Map<?, ?> map) || !map.containsKey("detail") || map.get("detail") == null) {
                return text;
            }
            Object detail = map.get("detail");
            if (detail instanceof String value) {
                return value;
            }
            return JSON.writeValueAsString(detail);
        } catch (IOException error) {
            return text;
        }
    }

    private static Tagged toTagged(Map<String, Object> body, boolean persist, String requestId) {
        String docId = body.get("doc_id") instanceof String value ? value : "";
        if (!persist || docId.isEmpty()) {
            docId = null;
        }
        String style = body.get("placeholder_style") instanceof String value ? value : "";
        if (style.isEmpty()) {
            style = "angle";
        }
        String balance = body.get("balance_credits") instanceof String value ? value : null;
        if (balance != null && balance.isEmpty()) {
            balance = null;
        }
        return new Tagged(
                body.get("text") instanceof String value ? value : "",
                docId,
                body.get("state") instanceof Map<?, ?> map ? cast(map) : null,
                intMap(body.get("entities_found")),
                balance,
                requestId,
                style,
                Boolean.TRUE.equals(body.get("keep_legal_form")),
                Boolean.TRUE.equals(body.get("health_data")),
                body.get("llm_review") instanceof Map<?, ?> map ? cast(map) : null);
    }

    private static Map<String, Integer> intMap(Object value) {
        Map<String, Integer> out = new LinkedHashMap<>();
        if (!(value instanceof Map<?, ?> map)) {
            return out;
        }
        for (Map.Entry<?, ?> entry : map.entrySet()) {
            if (entry.getValue() instanceof Number number) {
                out.put(String.valueOf(entry.getKey()), number.intValue());
            }
        }
        return out;
    }

    @SuppressWarnings("unchecked")
    private static Map<String, Object> cast(Map<?, ?> map) {
        return (Map<String, Object>) map;
    }

    private byte[] multipart(Options options, List<FilePart> files, String boundary) {
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        writeField(out, boundary, "persist", Boolean.toString(options.persist));
        writeField(out, boundary, "placeholder_style", options.placeholderStyle);
        writeField(out, boundary, "keep_legal_form", Boolean.toString(options.keepLegalForm));
        writeField(out, boundary, "health_data", Boolean.toString(options.healthData));
        writeField(out, boundary, "llm_review", Boolean.toString(options.llmReview));
        for (FilePart file : files) {
            writeFile(out, boundary, file);
        }
        out.writeBytes(("--" + boundary + "--\r\n").getBytes(StandardCharsets.UTF_8));
        return out.toByteArray();
    }

    private static void writeField(ByteArrayOutputStream out, String boundary, String name, String value) {
        String part = "--" + boundary + "\r\n"
                + "Content-Disposition: form-data; name=\"" + name + "\"\r\n\r\n"
                + value + "\r\n";
        out.writeBytes(part.getBytes(StandardCharsets.UTF_8));
    }

    private static void writeFile(ByteArrayOutputStream out, String boundary, FilePart file) {
        String filename = file.filename == null ? "file" : file.filename.replace("\"", "");
        String head = "--" + boundary + "\r\n"
                + "Content-Disposition: form-data; name=\"" + file.field + "\"; filename=\"" + filename + "\"\r\n"
                + "Content-Type: application/octet-stream\r\n\r\n";
        out.writeBytes(head.getBytes(StandardCharsets.UTF_8));
        out.writeBytes(file.content == null ? new byte[0] : file.content);
        out.writeBytes("\r\n".getBytes(StandardCharsets.UTF_8));
    }

    static String pathEscape(String value) {
        StringBuilder builder = new StringBuilder();
        byte[] bytes = value.getBytes(StandardCharsets.UTF_8);
        for (byte item : bytes) {
            int c = item & 0xff;
            if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
                    || c == '-' || c == '.' || c == '_' || c == '~') {
                builder.append((char) c);
            } else {
                builder.append('%');
                builder.append(Character.toUpperCase(Character.forDigit(c >>> 4, 16)));
                builder.append(Character.toUpperCase(Character.forDigit(c & 0xf, 16)));
            }
        }
        return builder.toString();
    }

    private record FilePart(String field, String filename, byte[] content) {
    }

    private record Exchange(Map<String, Object> body, String requestId) {
    }
}
