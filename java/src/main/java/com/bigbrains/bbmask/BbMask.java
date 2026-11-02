package com.bigbrains.bbmask;

import java.time.Duration;
import java.util.List;
import java.util.Map;

/**
 * Java client for the Data Mask anonymization service. It never calls a model.
 * A Spring service creates it in its own bean; the package does not depend on Spring.
 * anonymizeFile(path) reads a path; anonymizeBytes(filename, content) takes bytes.
 * Both names mean the same thing here and on {@link BBAnonym}.
 */
public final class BbMask {
    public static final String VERSION = "0.3.1";
    public static final String DEFAULT_BASE_URL = "https://app.bbmask.co";
    /** The engine waits 120 s; the client gives up after it so the refund/502 decision comes from the server. */
    public static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(150);
    public static final String STYLE_ANGLE = "angle";
    public static final String STYLE_BRACKET_TR = "bracket_tr";

    private static final Object LOCK = new Object();
    private static BBAnonym current;

    private BbMask() {
    }

    /** Sets up the process client. A second call replaces the first. An empty key throws 401. */
    public static BBAnonym init(String apiKey) {
        return init(apiKey, null, null);
    }

    public static BBAnonym init(String apiKey, String baseUrl, Duration timeout) {
        String key = apiKey == null ? "" : apiKey.trim();
        if (key.isEmpty()) {
            throw new BBError(401, "api_key is empty", null);
        }
        BBAnonym client = new BBAnonym(baseUrl, key, timeout);
        synchronized (LOCK) {
            current = client;
        }
        return client;
    }

    static void reset() {
        synchronized (LOCK) {
            current = null;
        }
    }

    private static BBAnonym require() {
        synchronized (LOCK) {
            if (current == null) {
                throw new BBError(401, "init was not called", null);
            }
            return current;
        }
    }

    public static Tagged anonymize(String text) {
        return anonymize(text, null);
    }

    public static Tagged anonymize(String text, Options options) {
        return require().anonymize(text, options);
    }

    public static Tagged anonymizeFile(String path) {
        return anonymizeFile(path, null);
    }

    /** Reads a path and sends one file. */
    public static Tagged anonymizeFile(String path, Options options) {
        return require().anonymizeFile(path, options);
    }

    public static Tagged anonymizeBytes(String filename, byte[] content) {
        return anonymizeBytes(filename, content, null);
    }

    /** When you already hold the bytes. */
    public static Tagged anonymizeBytes(String filename, byte[] content, Options options) {
        return require().anonymizeBytes(filename, content, options);
    }

    public static Tagged anonymizeDocuments(List<String> paths) {
        return anonymizeDocuments(paths, null);
    }

    public static Tagged anonymizeDocuments(List<String> paths, Options options) {
        return require().anonymizeDocuments(paths, options);
    }

    public static String restore(Object docIdOrState, String modelText) {
        return require().restore(docIdOrState, modelText);
    }

    public static Map<String, Object> getMap(String docId, boolean once) {
        return require().getMap(docId, once);
    }

    public static boolean delete(String docId) {
        return require().delete(docId);
    }

    public static MaskSession session() {
        return session(null);
    }

    public static MaskSession session(Options options) {
        return new MaskSession(require(), Options.resolve(options));
    }
}
