package com.bigbrains.bbmask;

/** Anonymize ve session seçenekleri. persist varsayılanı true, etiket biçimi angle. */
public final class Options {
    public boolean persist = true;
    public String placeholderStyle = "angle";
    public boolean keepLegalForm;
    public boolean healthData;
    public boolean llmReview;

    public Options persist(boolean value) {
        this.persist = value;
        return this;
    }

    public Options placeholderStyle(String value) {
        this.placeholderStyle = value;
        return this;
    }

    public Options keepLegalForm(boolean value) {
        this.keepLegalForm = value;
        return this;
    }

    public Options healthData(boolean value) {
        this.healthData = value;
        return this;
    }

    public Options llmReview(boolean value) {
        this.llmReview = value;
        return this;
    }

    public Options copy() {
        return new Options()
                .persist(persist)
                .placeholderStyle(placeholderStyle)
                .keepLegalForm(keepLegalForm)
                .healthData(healthData)
                .llmReview(llmReview);
    }

    public static Options resolve(Options options) {
        Options copy = options == null ? new Options() : options.copy();
        if (copy.placeholderStyle == null || copy.placeholderStyle.isBlank()) {
            copy.placeholderStyle = "angle";
        }
        if (!"angle".equals(copy.placeholderStyle) && !"bracket_tr".equals(copy.placeholderStyle)) {
            throw new BBError(400, "placeholder_style angle veya bracket_tr olmalı", null);
        }
        return copy;
    }
}
