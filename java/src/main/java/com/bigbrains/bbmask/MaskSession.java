package com.bigbrains.bbmask;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/** The docId or state stays inside the session. close deletes the mapping on exit. */
public final class MaskSession implements AutoCloseable {
    public final boolean persist;
    private final BBAnonym client;
    private final Options options;
    private final List<String> ids = new ArrayList<>();
    private String docId;
    private Map<String, Object> state;

    MaskSession(BBAnonym client, Options options) {
        this.client = client;
        this.options = options;
        this.persist = options.persist;
    }

    public Tagged anonymize(String text) {
        return remember(client.anonymize(text, options));
    }

    public Tagged anonymizeFile(String path) {
        return remember(client.anonymizeFile(path, options));
    }

    public Tagged anonymizeDocuments(List<String> paths) {
        return remember(client.anonymizeDocuments(paths, options));
    }

    public String restore(String modelText) {
        if (persist) {
            if (docId == null || docId.isEmpty()) {
                throw new BBError(400, "Nothing anonymized in this session", null);
            }
            return client.restore(docId, modelText);
        }
        if (state == null || state.isEmpty()) {
            throw new BBError(400, "No state in this session", null);
        }
        return client.restore(state, modelText);
    }

    @Override
    public void close() {
        if (!persist) {
            state = null;
            return;
        }
        for (String id : ids) {
            try {
                client.delete(id);
            } catch (BBError ignored) {
                // A failed delete does not fail the session.
            }
        }
        ids.clear();
    }

    private Tagged remember(Tagged tagged) {
        this.docId = tagged.docId();
        this.state = tagged.state();
        if (tagged.docId() != null && !tagged.docId().isEmpty()) {
            ids.add(tagged.docId());
        }
        return tagged;
    }
}
