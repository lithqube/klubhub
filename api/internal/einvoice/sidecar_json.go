package einvoice

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SidecarJSON renders the document as the JSON the e-invoice-eu server takes:
// a mirror of the Peppol UBL Invoice tree.
//
// Facts the spike established (plan section 8) and this encodes:
//   - every value is a string, amounts carry a sibling "@currencyID" key;
//   - repeatable nodes are arrays even with one element (PartyTaxScheme of the
//     seller, PaymentMeans, TaxTotal, TaxSubtotal, InvoiceLine);
//   - the CII output omits ApplicableHeaderTradeDelivery, which the schema
//     requires, unless a delivery date is sent, so BT-72 always goes out;
//   - a seller with only a Steuernummer must send it as BT-32 (tax scheme FC)
//     and as a seller identifier BT-29, or BR-CO-26 rejects the invoice;
//   - category O prints no percentage.
//
// Empty optional values are left out; nothing is ever null.
func (d Document) SidecarJSON() ([]byte, error) {
	cur := d.Currency
	money := func(m obj, key, value string) {
		m[key] = value
		m[key+"@currencyID"] = cur
	}

	inv := obj{
		"cbc:ID":                   d.Number,
		"cbc:InvoiceTypeCode":      d.TypeCode,
		"cbc:IssueDate":            d.IssueDate,
		"cbc:DocumentCurrencyCode": cur,
		"cac:Delivery":             obj{"cbc:ActualDeliveryDate": d.DeliveryDate},
	}
	set(inv, "cbc:DueDate", d.DueDate)
	set(inv, "cbc:BuyerReference", d.BuyerReference)
	if d.PurchaseOrderRef != "" {
		inv["cac:OrderReference"] = obj{"cbc:ID": d.PurchaseOrderRef}
	}
	if d.ContractRef != "" {
		inv["cac:ContractDocumentReference"] = obj{"cbc:ID": d.ContractRef}
	}
	if d.PrecedingNumber != "" {
		ref := obj{"cbc:ID": d.PrecedingNumber}
		set(ref, "cbc:IssueDate", d.PrecedingIssueDate)
		inv["cac:BillingReference"] = []any{obj{"cac:InvoiceDocumentReference": ref}}
	}
	if d.PaymentTerms != "" {
		inv["cac:PaymentTerms"] = obj{"cbc:Note": d.PaymentTerms}
	}

	inv["cac:AccountingSupplierParty"] = obj{"cac:Party": d.sellerParty()}
	inv["cac:AccountingCustomerParty"] = obj{"cac:Party": d.buyerParty()}

	if p := d.Payment; p != nil && p.IBAN != "" {
		account := obj{"cbc:ID": p.IBAN}
		if p.BIC != "" {
			account["cac:FinancialInstitutionBranch"] = obj{"cbc:ID": p.BIC}
		}
		means := obj{"cbc:PaymentMeansCode": "58", "cac:PayeeFinancialAccount": account}
		set(means, "cbc:PaymentID", p.Reference)
		inv["cac:PaymentMeans"] = []any{means}
	}

	subtotals := make([]any, 0, len(d.Taxes))
	for _, t := range d.Taxes {
		sub := obj{"cac:TaxCategory": t.Category.json(true)}
		money(sub, "cbc:TaxableAmount", t.Taxable)
		money(sub, "cbc:TaxAmount", t.Tax)
		subtotals = append(subtotals, sub)
	}
	taxTotal := obj{"cac:TaxSubtotal": subtotals}
	money(taxTotal, "cbc:TaxAmount", d.Totals.Tax)
	inv["cac:TaxTotal"] = []any{taxTotal}

	totals := obj{}
	money(totals, "cbc:LineExtensionAmount", d.Totals.Lines)
	money(totals, "cbc:TaxExclusiveAmount", d.Totals.TaxExclusive)
	money(totals, "cbc:TaxInclusiveAmount", d.Totals.TaxInclusive)
	money(totals, "cbc:PayableAmount", d.Totals.Payable)
	inv["cac:LegalMonetaryTotal"] = totals

	lines := make([]any, 0, len(d.Lines))
	for _, l := range d.Lines {
		line := obj{
			"cbc:ID":                        strconv.Itoa(l.ID),
			"cbc:InvoicedQuantity":          l.Quantity,
			"cbc:InvoicedQuantity@unitCode": l.UnitCode,
			"cac:Item": obj{
				"cbc:Name":                  l.Name,
				"cac:ClassifiedTaxCategory": l.Category.json(false),
			},
		}
		money(line, "cbc:LineExtensionAmount", l.Net)
		price := obj{}
		money(price, "cbc:PriceAmount", l.UnitPrice)
		line["cac:Price"] = price
		lines = append(lines, line)
	}
	inv["cac:InvoiceLine"] = lines

	return json.Marshal(obj{"ubl:Invoice": inv})
}

type obj = map[string]any

func set(m obj, key, value string) {
	if strings.TrimSpace(value) != "" {
		m[key] = value
	}
}

// json renders a VAT category. The exemption reason belongs to the breakdown
// (BT-120/121), never to a line.
func (c Category) json(breakdown bool) obj {
	m := obj{"cbc:ID": c.Code, "cac:TaxScheme": obj{"cbc:ID": "VAT"}}
	if c.HasPercent {
		m["cbc:Percent"] = c.Percent
	}
	if breakdown {
		set(m, "cbc:TaxExemptionReasonCode", c.ExemptionCode)
		set(m, "cbc:TaxExemptionReason", c.ExemptionReason)
	}
	return m
}

func (p Party) address() obj {
	a := obj{"cac:Country": obj{"cbc:IdentificationCode": p.Country}}
	set(a, "cbc:StreetName", p.Street)
	set(a, "cbc:AdditionalStreetName", p.Street2)
	set(a, "cbc:CityName", p.City)
	set(a, "cbc:PostalZone", p.Postal)
	set(a, "cbc:CountrySubentity", p.Region)
	return a
}

func (p Party) base() obj {
	m := obj{
		"cac:PostalAddress":    p.address(),
		"cac:PartyLegalEntity": obj{"cbc:RegistrationName": p.Name},
	}
	if p.Email != "" {
		m["cbc:EndpointID"] = p.Email
		m["cbc:EndpointID@schemeID"] = "EM"
	}
	if p.TradingName != "" {
		m["cac:PartyName"] = obj{"cbc:Name": p.TradingName}
	}
	return m
}

func (d Document) sellerParty() obj {
	p := d.Seller
	m := p.base()

	// BR-O-02: a document with a category O line carries no VAT IDs.
	vatID := p.VATID
	if d.outsideScope() {
		vatID = ""
	}

	// The seller's tax schemes are an array: VAT ID (BT-31) and/or Steuernummer
	// (BT-32, scheme FC).
	schemes := []any{}
	if vatID != "" {
		schemes = append(schemes, obj{"cbc:CompanyID": vatID, "cac:TaxScheme": obj{"cbc:ID": "VAT"}})
	}
	if p.TaxNumber != "" {
		schemes = append(schemes, obj{"cbc:CompanyID": p.TaxNumber, "cac:TaxScheme": obj{"cbc:ID": "FC"}})
	}
	m["cac:PartyTaxScheme"] = schemes
	// BR-CO-26: without a VAT ID the seller must carry an identifier.
	if vatID == "" && p.TaxNumber != "" {
		m["cac:PartyIdentification"] = []any{obj{"cbc:ID": p.TaxNumber}}
	}

	contact := obj{}
	set(contact, "cbc:Name", p.ContactName)
	set(contact, "cbc:Telephone", p.Phone)
	set(contact, "cbc:ElectronicMail", p.Email)
	if len(contact) > 0 {
		m["cac:Contact"] = contact
	}
	return m
}

func (d Document) buyerParty() obj {
	p := d.Buyer
	m := p.base()
	if p.VATID != "" && !d.outsideScope() { // BR-O-02
		m["cac:PartyTaxScheme"] = obj{"cbc:CompanyID": p.VATID, "cac:TaxScheme": obj{"cbc:ID": "VAT"}}
	}
	return m
}
