package models

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// ─── PSBT builders ──────────────────────────────────────────────────────────

// testPkScript builds a minimal 22-byte P2WPKH program: OP_0 PUSH20 <20 bytes>.
func testPkScript() []byte {
	script := make([]byte, 22)
	script[0] = 0x00
	script[1] = 0x14
	return script
}

// testPSBTBase64 returns a real minimal PSBT with one input and one output.
func testPSBTBase64(t *testing.T) string {
	t.Helper()
	packet, err := psbt.New(
		[]*wire.OutPoint{{Hash: chainhash.Hash{0x01}, Index: 0}},
		[]*wire.TxOut{{Value: 50000, PkScript: testPkScript()}},
		2,
		0,
		[]uint32{wire.MaxTxInSequenceNum},
	)
	if err != nil {
		t.Fatalf("psbt.New failed: %v", err)
	}
	b64, err := packet.B64Encode()
	if err != nil {
		t.Fatalf("B64Encode failed: %v", err)
	}
	return b64
}

// testBigPSBTBase64 returns a PSBT whose base64 form is longer than 2048 chars.
func testBigPSBTBase64(t *testing.T) string {
	t.Helper()
	const extraInputs = 40

	inputs := make([]*wire.OutPoint, 0, extraInputs+1)
	sequences := make([]uint32, 0, extraInputs+1)
	inputs = append(inputs, &wire.OutPoint{Hash: chainhash.Hash{0x01}, Index: 0})
	sequences = append(sequences, wire.MaxTxInSequenceNum)
	for i := 0; i < extraInputs; i++ {
		inputs = append(inputs, &wire.OutPoint{
			Hash:  chainhash.Hash{0x02, byte(i)},
			Index: uint32(i),
		})
		sequences = append(sequences, wire.MaxTxInSequenceNum)
	}

	packet, err := psbt.New(
		inputs,
		[]*wire.TxOut{{Value: 50000, PkScript: testPkScript()}},
		2,
		0,
		sequences,
	)
	if err != nil {
		t.Fatalf("psbt.New failed: %v", err)
	}
	b64, err := packet.B64Encode()
	if err != nil {
		t.Fatalf("B64Encode failed: %v", err)
	}
	if len(b64) <= 2048 {
		t.Fatalf("big PSBT base64 length = %d, want > 2048", len(b64))
	}
	return b64
}

func pasteMsg(s string) tea.PasteMsg {
	return tea.PasteMsg{Content: s}
}

// ─── Tests ─────────────────────────────────────────────────────────────────

// TestSend_PasteBase64PSBTOnAddressStep covers pasting a base64 PSBT (with
// surrounding whitespace) directly into the address input.
func TestSend_PasteBase64PSBTOnAddressStep(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)
	big := testBigPSBTBase64(t)

	updated, cmd := m.Update(pasteMsg("  \n" + big + "\n"))
	m = asSendModel(t, updated)

	fileMsg := requirePSBTFileMsg(t, runCmd(cmd))
	if fileMsg.Err != nil {
		t.Fatalf("PSBTFileMsg.Err = %v, want nil", fileMsg.Err)
	}
	if fileMsg.Base64 != big {
		t.Fatalf("PSBTFileMsg.Base64 length = %d, want %d (exact match)", len(fileMsg.Base64), len(big))
	}

	updated, _ = m.Update(fileMsg)
	m = asSendModel(t, updated)
	if m.step != SendStepReview {
		t.Fatalf("step = %v, want SendStepReview after importing a pasted PSBT", m.step)
	}
	if !m.showPSBTExport {
		t.Fatalf("showPSBTExport = false, want true after importing a pasted PSBT")
	}
	if got := m.addressInput.Value(); got != "" {
		t.Fatalf("addressInput.Value() length = %d, want empty", len(got))
	}
}

// TestSend_PasteHexPSBTOnAddressStep covers pasting the hex form of a PSBT.
func TestSend_PasteHexPSBTOnAddressStep(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)
	small := testPSBTBase64(t)

	raw, err := base64.StdEncoding.DecodeString(small)
	if err != nil {
		t.Fatalf("failed to decode test PSBT base64: %v", err)
	}

	updated, cmd := m.Update(pasteMsg(hex.EncodeToString(raw)))
	m = asSendModel(t, updated)

	fileMsg := requirePSBTFileMsg(t, runCmd(cmd))
	if fileMsg.Err != nil {
		t.Fatalf("PSBTFileMsg.Err = %v, want nil", fileMsg.Err)
	}
	if fileMsg.Base64 != small {
		t.Fatalf("PSBTFileMsg.Base64 length = %d, want %d (exact match)", len(fileMsg.Base64), len(small))
	}
}

// TestSend_PastePSBTInFileBrowser covers pasting a multi-line base64 PSBT while
// the file browser is open.
func TestSend_PastePSBTInFileBrowser(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)
	small := testPSBTBase64(t)

	updated, _ := m.Update(keyRunes("i"))
	m = asSendModel(t, updated)
	if !m.showFileBrowser {
		t.Fatalf("pressing i at the address step did not open the file browser")
	}

	var lines []string
	for i := 0; i < len(small); i += 40 {
		end := i + 40
		if end > len(small) {
			end = len(small)
		}
		lines = append(lines, small[i:end])
	}

	updated, cmd := m.Update(pasteMsg(strings.Join(lines, "\n")))
	m = asSendModel(t, updated)

	fileMsg := requirePSBTFileMsg(t, runCmd(cmd))
	if fileMsg.Err != nil {
		t.Fatalf("PSBTFileMsg.Err = %v, want nil", fileMsg.Err)
	}
	if fileMsg.Base64 != small {
		t.Fatalf("PSBTFileMsg.Base64 length = %d, want %d (exact match)", len(fileMsg.Base64), len(small))
	}
}

// TestSend_PasteInvalidPSBTInFileBrowser covers pasting garbage while the file
// browser is open: it must set an error without echoing the pasted data.
func TestSend_PasteInvalidPSBTInFileBrowser(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)

	updated, _ := m.Update(keyRunes("i"))
	m = asSendModel(t, updated)
	if !m.showFileBrowser {
		t.Fatalf("pressing i at the address step did not open the file browser")
	}

	pasted := "cHNidP8BAGARBAGE!!"
	updated, _ = m.Update(pasteMsg(pasted))
	m = asSendModel(t, updated)

	importErr := m.err
	if importErr == nil {
		importErr = m.psbtExportErr
	}
	if importErr == nil {
		t.Fatalf("pasting an invalid PSBT set no error (want m.err or psbtExportErr)")
	}
	msg := importErr.Error()
	if !strings.Contains(msg, "PSBT") {
		t.Fatalf("error message does not mention PSBT (message length %d)", len(msg))
	}
	if strings.Contains(msg, pasted) {
		t.Fatalf("error message echoes the pasted PSBT data (message length %d, pasted length %d)", len(msg), len(pasted))
	}
}

// TestSend_PastePSBTIntoFilePathInput covers pasting a base64 PSBT into the
// manual path input opened from the file browser.
func TestSend_PastePSBTIntoFilePathInput(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)
	small := testPSBTBase64(t)

	updated, _ := m.Update(keyRunes("i"))
	m = asSendModel(t, updated)
	updated, _ = m.Update(keyRunes("p"))
	m = asSendModel(t, updated)
	if !m.showFileInput {
		t.Fatalf("pressing p in the file browser did not open the manual path input")
	}

	updated, cmd := m.Update(pasteMsg(small))
	m = asSendModel(t, updated)

	fileMsg := requirePSBTFileMsg(t, runCmd(cmd))
	if fileMsg.Base64 != small {
		t.Fatalf("PSBTFileMsg.Base64 length = %d, want %d (exact match)", len(fileMsg.Base64), len(small))
	}
}

// TestSend_URInputAcceptsBase64 covers pasting a plain base64 PSBT into the UR
// import overlay, including pasted line wrapping.
func TestSend_URInputAcceptsBase64(t *testing.T) {
	m := newTestSend(t)
	m.walletManager = mockForPaste(t)
	small := testPSBTBase64(t)
	big := testBigPSBTBase64(t)

	m.step = SendStepReview
	m.showPSBTExport = true
	m.psbtBase64 = small

	updated, _ := m.Update(keyRunes("u"))
	m = asSendModel(t, updated)
	if !m.showURInput {
		t.Fatalf("pressing u in the PSBT export view did not open the UR input")
	}

	mid := len(big) / 2
	wrapped := big[:mid] + "\n" + big[mid:]
	updated, _ = m.Update(pasteMsg(wrapped))
	m = asSendModel(t, updated)

	updated, _ = m.Update(keyCode(tea.KeyEnter))
	m = asSendModel(t, updated)

	if m.psbtExportErr != nil {
		t.Fatalf("psbtExportErr is set (message length %d), want nil after importing a base64 PSBT", len(m.psbtExportErr.Error()))
	}
	if m.psbtBase64 != big {
		t.Fatalf("psbtBase64 length = %d, want %d (exact match)", len(m.psbtBase64), len(big))
	}
	if m.showURInput {
		t.Fatalf("showURInput = true, want false after a successful import")
	}
}

// TestSend_URInputPasteStripsWhitespace covers pasting a base64 PSBT into the
// UR import overlay with line wraps and spaces: the input must hold the PSBT
// text without any whitespace.
func TestSend_URInputPasteStripsWhitespace(t *testing.T) {
	m := newTestSend(t)
	small := testPSBTBase64(t)

	m.step = SendStepReview
	m.showPSBTExport = true
	m.psbtBase64 = small

	updated, _ := m.Update(keyRunes("u"))
	m = asSendModel(t, updated)
	if !m.showURInput {
		t.Fatalf("pressing u in the PSBT export view did not open the UR input")
	}

	pasted := small[:10] + " \n " + small[10:30] + "\r\n\t" + small[30:]
	updated, _ = m.Update(pasteMsg(pasted))
	m = asSendModel(t, updated)

	got := m.urInput.Value()
	if i := strings.IndexFunc(got, unicode.IsSpace); i >= 0 {
		t.Fatalf("urInput.Value() contains whitespace at byte %d (length %d)", i, len(got))
	}
	if got != small {
		t.Fatalf("urInput.Value() length = %d, want %d (exact match)", len(got), len(small))
	}
}
