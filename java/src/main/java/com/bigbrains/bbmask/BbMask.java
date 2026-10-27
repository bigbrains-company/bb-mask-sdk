package com.bigbrains.bbmask;

import java.time.Duration;
import java.util.List;
import java.util.Map;

/**
 * Data Mask anonimleştirme servisinin Java istemcisi. Model çağırmaz.
 * Spring servisi bu sınıfı kendi bean'inde kurar; paket Spring'e bağlanmaz.
 */
public final class BbMask {
    public static final String VERSION = "0.2.0";
    public static final String DEFAULT_BASE_URL = "https://app.bbmask.co";
    public static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(120);
    public static final String STYLE_ANGLE = "angle";
    public static final String STYLE_BRACKET_TR = "bracket_tr";

    private static final Object LOCK = new Object();
    private static BBAnonym current;

    private BbMask() {
    }

    /** Süreç istemcisini kurar. İkinci çağrı öncekinin yerine geçer. Boş anahtar 401 fırlatır. */
    public static BBAnonym init(String apiKey) {
        return init(apiKey, null, null);
    }

    public static BBAnonym init(String apiKey, String baseUrl, Duration timeout) {
        String key = apiKey == null ? "" : apiKey.trim();
        if (key.isEmpty()) {
            throw new BBError(401, "api_key boş", null);
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
                throw new BBError(401, "init çağrılmadı", null);
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

    /** Yolu okur. Bayt sizdeyse {@link BBAnonym#anonymizeFile(String, byte[], Options)} kullanın. */
    public static Tagged anonymizeFile(String path, Options options) {
        return require().anonymizePath(path, options);
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

    public static String complete(String text) {
        return require().complete(text);
    }

    public static String completeFile(String path) {
        return require().completeFile(path);
    }

    public static String completeDocuments(List<String> paths) {
        return require().completeDocuments(paths);
    }
}
