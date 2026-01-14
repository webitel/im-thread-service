### **BACKLOG.md**

#### **[RELIABILITY] IMPLEMENT CIRCUIT BREAKER FOR STORAGE CLIENT**

**TASK:** Wrap `storage` gRPC calls in `MediaProcessor` using [sony/gobreaker](https://github.com/sony/gobreaker).

**RATIONALE:**

* **[FAIL_FAST]:** Prevents the service from hanging when `storage` is down or slow, protecting resources (goroutines/DB connections).
* **[CASCADING_FAILURE]:** Stops "retry storms" and avoids spreading instability from the `storage` service to `im-thread-service`.
* **[SELF_RECOVERY]:** Automatically probes the external service and restores traffic once it becomes healthy.