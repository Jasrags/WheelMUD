package telnet

// ASCII control characters.
const (
	ASCII_NULL  byte = iota // ( Null character )
	ASCII_SOH               // ( Start of Header )
	ASCII_STX               // ( Start of Text )
	ASCII_ETX               // ( End of Text, hearts card suit )
	ASCII_EOT               // ( End of Transmission, diamonds card suit )
	ASCII_ENQ               // ( Enquiry, clubs card suit )
	ASCII_ACK               // ( Acknowledgement, spade card suit )
	ASCII_BEL               // ( Bell )
	ASCII_BS                // ( Backspace )
	ASCII_HT                // ( Horizontal Tab )
	ASCII_LF                // ( Line feed )
	ASCII_VT                // ( Vertical Tab, male symbol, symbol for Mars )
	ASCII_FF                // ( Form feed, female symbol, symbol for Venus )
	ASCII_CR                // ( Carriage return )
	ASCII_SO                // ( Shift Out )
	ASCII_SI                // ( Shift In )
	ASCII_DLE               // ( Data link escape )
	ASCII_DC1               // ( Device control 1 )
	ASCII_DC2               // ( Device control 2 )
	ASCII_DC3               // ( Device control 3 )
	ASCII_DC4               // ( Device control 4 )
	ASCII_NAK               // ( NAK Negative-acknowledge )
	ASCII_SYN               // ( Synchronous idle )
	ASCII_ETB               // ( End of trans. block )
	ASCII_CAN               // ( Cancel )
	ASCII_EM                // ( End of medium )
	ASCII_SUB               // ( Substitute )
	ASCII_ESC               // ( Escape )
	ASCII_FS                // ( File separator )
	ASCII_GS                // ( Group separator )
	ASCII_RS                // ( Record separator )
	ASCII_US                // ( Unit separator )
	ASCII_SPACE byte = 32   // ( Space )
	ASCII_DEL   byte = 127  // ( Delete )
)

func ASCIIString(char byte) string {
	switch char {
	case ASCII_NULL:
		return "NUL"
	case ASCII_SOH:
		return "SOH"
	case ASCII_STX:
		return "STX"
	case ASCII_ETX:
		return "ETX"
	case ASCII_EOT:
		return "EOT"
	case ASCII_ENQ:
		return "ENQ"
	case ASCII_ACK:
		return "ACK"
	case ASCII_BEL:
		return "BEL"
	case ASCII_BS:
		return "BS"
	case ASCII_HT:
		return "HT"
	case ASCII_LF:
		return "LF"
	case ASCII_VT:
		return "VT"
	case ASCII_FF:
		return "FF"
	case ASCII_CR:
		return "CR"
	case ASCII_SO:
		return "SO"
	case ASCII_SI:
		return "SI"
	case ASCII_DLE:
		return "DLE"
	case ASCII_DC1:
		return "DC1"
	case ASCII_DC2:
		return "DC2"
	case ASCII_DC3:
		return "DC3"
	case ASCII_DC4:
		return "DC4"
	case ASCII_NAK:
		return "NAK"
	case ASCII_SYN:
		return "SYN"
	case ASCII_ETB:
		return "ETB"
	case ASCII_CAN:
		return "CAN"
	case ASCII_EM:
		return "EM"
	case ASCII_SUB:
		return "SUB"
	case ASCII_ESC:
		return "ESC"
	case ASCII_FS:
		return "FS"
	case ASCII_GS:
		return "GS"
	case ASCII_RS:
		return "RS"
	case ASCII_US:
		return "US"
	case ASCII_DEL:
		return "DEL"
	default:
		return "UNKNOWN: " + string(char)
	}
}

// func isASCIITextCharacter(b byte) bool {
// 	return b >= 32 && b <= 126
// }
