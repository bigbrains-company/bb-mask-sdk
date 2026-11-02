package com.bigbrains.bbmask;

import java.util.Map;

/** Result of an anonymize call. */
public record Tagged(
        String text,
        String docId,
        Map<String, Object> state,
        Map<String, Integer> entitiesFound,
        String balanceCredits,
        String requestId,
        String placeholderStyle,
        boolean keepLegalForm,
        boolean healthData,
        Map<String, Object> llmReview) {

    public Tagged {
        if (text == null) {
            text = "";
        }
        if (entitiesFound == null) {
            entitiesFound = Map.of();
        }
        if (placeholderStyle == null || placeholderStyle.isEmpty()) {
            placeholderStyle = "angle";
        }
    }
}
