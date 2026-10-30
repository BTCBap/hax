package models

import (
	"fmt"
	"strings"

	"github.com/BTCBap/hax/internal/api"
	"github.com/BTCBap/hax/internal/ui/styles"
	"github.com/BTCBap/hax/internal/wallet"
	tea "github.com/charmbracelet/bubbletea"
)

type bumpMethod int

const (
	bumpRBF  bumpMethod = iota // replace the transaction
	bumpCPFP                   // spend its output in a child
)

// feeBumpState is set when the send screen speeds up a pending
// transaction instead of making a new payment.
type feeBumpState struct {
	txID   string
	info   *wallet.FeeBumpInfo
	method bumpMethod
	// presetRate is set when the tray chose a rate. The fee step is
	// pre-filled with it and the review is built automatically.
	presetRate float64
}

// FeeBumpInfoMsg carries the analysis of the transaction to speed up.
type FeeBumpInfoMsg struct {
	Info *wallet.FeeBumpInfo
	Err  error
}

// NavigateToFeeBump opens the send screen to speed up txid.
func NavigateToFeeBump(txid string) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: StateSend, FeeBumpTxID: txid}
	}
}

// NavigateToFeeBumpWithRate opens the send screen with a fee rate already
// chosen by the inline speed-up tray.
func NavigateToFeeBumpWithRate(txid string, rate float64) tea.Cmd {
	return func() tea.Msg {
		return NavigateMsg{Target: StateSend, FeeBumpTxID: txid, FeeBumpRate: rate}
	}
}

// NewFeeBumpModel is the send flow for speeding up txid: choose a fee,
// review, sign and broadcast, reusing the normal send steps.
func NewFeeBumpModel(walletManager WalletService, txid string, cachedBalance api.Satoshi, cachedUTXOs []api.UTXO) SendModel {
	return newFeeBumpModel(walletManager, txid, 0, cachedBalance, cachedUTXOs)
}

// NewFeeBumpModelWithRate is NewFeeBumpModel with the fee step pre-filled
// and advanced to review once the transaction analysis allows building it.
func NewFeeBumpModelWithRate(walletManager WalletService, txid string, rate float64, cachedBalance api.Satoshi, cachedUTXOs []api.UTXO) SendModel {
	return newFeeBumpModel(walletManager, txid, rate, cachedBalance, cachedUTXOs)
}

func newFeeBumpModel(walletManager WalletService, txid string, rate float64, cachedBalance api.Satoshi, cachedUTXOs []api.UTXO) SendModel {
	m := NewSendModel(walletManager, cachedBalance, cachedUTXOs)
	m.bump = &feeBumpState{txID: txid, presetRate: rate}
	m.addressInput.Blur()
	m.step = SendStepFee
	m.cursor = cursorFromFeeLevel(FeeLevelMedium)
	if rate > 0 {
		m.feeLevel = FeeLevelCustom
		m.customFeeRate = rate
		m.cursor = cursorFromFeeLevel(FeeLevelCustom)
	}
	return m
}

func (m SendModel) analyzeFeeBump() tea.Cmd {
	walletMgr, txid := m.walletManager, m.bump.txID
	return func() tea.Msg {
		info, err := walletMgr.AnalyzeFeeBump(txid)
		return FeeBumpInfoMsg{Info: info, Err: err}
	}
}

func (m *SendModel) handleFeeBumpInfo(msg FeeBumpInfoMsg) {
	if m.bump == nil {
		return
	}
	if msg.Err != nil {
		m.err = fmt.Errorf("can't speed up this transaction: %v", msg.Err)
		return
	}
	info := msg.Info
	m.bump.info = info
	switch {
	case info.CanRBF:
		m.bump.method = bumpRBF
	case info.CanCPFP:
		m.bump.method = bumpCPFP
	default:
		m.err = fmt.Errorf("can't speed up this transaction: it can't be "+
			"replaced (%s) and %s", info.RBFReason, info.CPFPReason)
	}
	m.estimatedSize = info.VSize
}

// bumpReady reports whether a fee bump can be built.
func (m SendModel) bumpReady() bool {
	if m.bump == nil || m.bump.info == nil {
		return false
	}
	if m.bump.method == bumpRBF {
		return m.bump.info.CanRBF
	}
	return m.bump.info.CanCPFP
}

// bumpMinRate is the lowest feerate the chosen method accepts.
func (m SendModel) bumpMinRate() float64 {
	info := m.bump.info
	if m.bump.method == bumpRBF {
		return info.MinRBFRate
	}
	return info.FeeRate + 1
}

// toggleBumpMethod switches between RBF and CPFP when both work.
func (m *SendModel) toggleBumpMethod() {
	if m.bump == nil || m.bump.info == nil {
		return
	}
	if m.bump.method == bumpRBF && m.bump.info.CanCPFP {
		m.bump.method = bumpCPFP
	} else if m.bump.method == bumpCPFP && m.bump.info.CanRBF {
		m.bump.method = bumpRBF
	}
	m.err = nil
}

// checkBumpRate validates the chosen feerate before building.
func (m SendModel) checkBumpRate(rate float64) error {
	if !m.bumpReady() {
		if m.bump.info == nil && m.err == nil {
			return fmt.Errorf("still looking up the transaction")
		}
		return fmt.Errorf("can't speed up this transaction")
	}
	if min := m.bumpMinRate(); rate < min {
		return fmt.Errorf("pick at least %s sat/vB (c for custom)",
			formatFeeRate(roundUpRate(min)))
	}
	return nil
}

func roundUpRate(r float64) float64 {
	return float64(int64(r*10+0.999)) / 10
}

// startPresetFeeBump builds the review for the tray's chosen rate. Any
// error leaves the fee step open with the rate still filled in.
func (m *SendModel) startPresetFeeBump() tea.Cmd {
	if m.bump == nil || m.bump.presetRate <= 0 || !m.bumpReady() {
		return nil
	}
	rate := m.bump.presetRate
	if err := m.checkBumpRate(rate); err != nil {
		m.err = err
		return nil
	}
	if rate > feeHardCap {
		m.err = fmt.Errorf("fee rate too high (maximum %.0f sat/vB)", feeHardCap)
		return nil
	}
	if rate > feeSoftCap {
		m.err = fmt.Errorf("fee rate is above the safety limit; lower it on the fee step")
		return nil
	}
	m.err = nil
	m.step = SendStepPreviewLoading
	return tea.Batch(m.previewFeeBump(rate), m.spinner.Tick)
}

// previewFeeBump builds the replacement or child at feeRate.
func (m SendModel) previewFeeBump(feeRate float64) tea.Cmd {
	walletMgr, txid, method := m.walletManager, m.bump.txID, m.bump.method
	return func() tea.Msg {
		var res *wallet.PSBTResult
		var err error
		if method == bumpRBF {
			res, err = walletMgr.BuildRBF(txid, feeRate)
		} else {
			res, err = walletMgr.BuildCPFP(txid, feeRate)
		}
		return TxPreviewMsg{Result: res, Err: err}
	}
}

// bumpRecipients are the payments a replacement keeps, for the review.
func (m SendModel) bumpRecipients() []Recipient {
	if m.bump == nil || m.bump.info == nil || m.bump.method != bumpRBF {
		return nil
	}
	var out []Recipient
	for _, r := range m.bump.info.Recipients {
		out = append(out, Recipient{Address: r.Address, Amount: r.Amount})
	}
	return out
}

// sendTitle is the screen title: a payment or a speed-up.
func (m SendModel) sendTitle() string {
	if m.bump != nil {
		return styles.TitleStyle.Render("⚡ Speed Up Transaction")
	}
	return styles.TitleStyle.Render("📤 Send Bitcoin")
}

// renderBumpSummary describes the pending transaction and the method.
func (m SendModel) renderBumpSummary(boxWidth int) string {
	rows := []string{renderRow("Transaction:", shortTxIDUI(m.bump.txID))}
	info := m.bump.info
	if info == nil {
		if m.err == nil {
			rows = append(rows, styles.DimText.Render(m.spinner.View()+" Looking it up..."))
		}
		return styles.InfoBoxStyle.Width(boxWidth).Render(strings.Join(rows, "\n"))
	}
	rows = append(rows, renderRow("Pays now:", fmt.Sprintf("%d sats · %s sat/vB · %d vB",
		info.Fee, formatFeeRate(roundRate(info.FeeRate)), info.VSize)))
	if m.bumpReady() {
		method := "replace it with a higher fee (RBF)"
		other, otherOK := "use a child (CPFP)", info.CanCPFP
		if m.bump.method == bumpCPFP {
			method = "spend your output with a child (CPFP)"
			other, otherOK = "replace it (RBF)", info.CanRBF
		}
		rows = append(rows, renderRow("Method:", method))
		if otherOK {
			rows = append(rows, styles.DimText.Render("  m: "+other+" instead"))
		} else if m.bump.method == bumpCPFP {
			rows = append(rows, styles.DimText.Render("  RBF not possible: "+info.RBFReason))
		}
		label := "New fee rate:"
		if m.bump.method == bumpCPFP {
			label = "Package rate:"
		}
		rows = append(rows, renderRow(label, "at least "+
			formatFeeRate(roundUpRate(m.bumpMinRate()))+" sat/vB"))
	}
	return styles.InfoBoxStyle.Width(boxWidth).Render(strings.Join(rows, "\n"))
}

// bumpFeeAt estimates what speeding up costs at rate, for the fee menu.
func (m SendModel) bumpFeeAt(rate float64) api.Satoshi {
	info := m.bump.info
	if info == nil {
		return 0
	}
	if m.bump.method == bumpCPFP {
		child := m.estimateTxVSize(1, 1)
		fee := api.Satoshi(rate*float64(info.VSize+child)+0.999) - info.Fee
		if fee < api.Satoshi(child) {
			fee = api.Satoshi(child)
		}
		return fee
	}
	fee := api.Satoshi(rate*float64(info.VSize) + 0.999)
	if min := info.Fee + api.Satoshi(info.VSize); fee < min {
		fee = min
	}
	return fee
}
