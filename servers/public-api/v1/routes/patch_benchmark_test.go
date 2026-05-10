package routes

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
)

func BenchmarkPatchValueRevisionModes(b *testing.B) {
	for _, mode := range []string{"off", "current", "history"} {
		b.Run(mode, func(b *testing.B) {
			setupRoutesTest(b)

			save := perform(AsyncSave, http.MethodPost, "/save/docs/doc1", bytes.NewBufferString("hello"), nil)
			if save.Code != http.StatusOK {
				b.Fatalf("save status: %d body=%s", save.Code, save.Body.String())
			}
			if mode != "off" {
				policy := perform(Revision, http.MethodPost, "/revision/docs/doc1", bytes.NewBufferString(fmt.Sprintf(`{"mode":%q}`, mode)), nil)
				if policy.Code != http.StatusOK {
					b.Fatalf("revision policy status: %d body=%s", policy.Code, policy.Body.String())
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var body string
				if mode == "off" {
					body = `{"ops":[{"offset":0,"insert":"x"}]}`
				} else {
					body = fmt.Sprintf(`{"base_rev":%d,"ops":[{"offset":0,"insert":"x"}]}`, i)
				}
				resp := perform(PatchValue, http.MethodPost, "/patch/docs/doc1", bytes.NewBufferString(body), nil)
				if resp.Code != http.StatusOK {
					b.Fatalf("patch status: %d body=%s", resp.Code, resp.Body.String())
				}
			}
		})
	}
}
