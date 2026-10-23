package com.bigbrains.bbmask;

/** Servis veya istemci hatası. getMessage() ayrıntı metnidir. */
public final class BBError extends RuntimeException {
    private final int statusCode;
    private final String detail;
    private final String requestId;

    public BBError(int statusCode, String detail, String requestId) {
        super(detail);
        this.statusCode = statusCode;
        this.detail = detail;
        this.requestId = requestId;
    }

    public int getStatusCode() {
        return statusCode;
    }

    public String getDetail() {
        return detail;
    }

    public String getRequestId() {
        return requestId;
    }
}
