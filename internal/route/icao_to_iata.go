package route

// icaoToIATA maps common ICAO airport codes to IATA codes for display.
// This covers the most common US and international airports. Unknown codes
// pass through as-is (with the leading K stripped for US airports).
var icaoToIATA = map[string]string{
	// US major airports
	"KATL": "ATL", "KBNA": "BNA", "KBOS": "BOS", "KBWI": "BWI",
	"KCLE": "CLE", "KCLT": "CLT", "KCMH": "CMH", "KCVG": "CVG",
	"KDAL": "DAL", "KDCA": "DCA", "KDEN": "DEN", "KDFW": "DFW",
	"KDTW": "DTW", "KELP": "ELP", "KEWR": "EWR", "KFLL": "FLL",
	"KGEG": "GEG", "KHNL": "HNL", "KHOU": "HOU", "KIAD": "IAD",
	"KIAH": "IAH", "KIND": "IND", "KJAC": "JAC", "KJAX": "JAX",
	"KJFK": "JFK", "KLAS": "LAS", "KLAX": "LAX", "KLGA": "LGA",
	"KLIT": "LIT", "KMCI": "MCI", "KMCO": "MCO", "KMDW": "MDW",
	"KMEM": "MEM", "KMIA": "MIA", "KMKE": "MKE", "KMSN": "MSN",
	"KMSP": "MSP", "KMSY": "MSY", "KOAK": "OAK", "KOKC": "OKC",
	"KOMA": "OMA", "KONT": "ONT", "KORD": "ORD", "KPBI": "PBI",
	"KPDX": "PDX", "KPHL": "PHL", "KPHX": "PHX", "KPIT": "PIT",
	"KPSP": "PSP", "KPVD": "PVD", "KRDU": "RDU", "KRIC": "RIC",
	"KRNO": "RNO", "KRSW": "RSW", "KSAN": "SAN", "KSAT": "SAT",
	"KSDF": "SDF", "KSEA": "SEA", "KSFO": "SFO", "KSJC": "SJC",
	"KSLC": "SLC", "KSMF": "SMF", "KSNA": "SNA", "KSTL": "STL",
	"KTPA": "TPA", "KTUS": "TUS",
	// Alaska/Hawaii
	"PANC": "ANC", "PAFA": "FAI", "PHNL": "HNL", "PHOG": "OGG",
	// Canada
	"CYUL": "YUL", "CYYZ": "YYZ", "CYVR": "YVR", "CYOW": "YOW",
	"CYWG": "YWG", "CYEG": "YEG", "CYYC": "YYC",
	// Europe
	"EGLL": "LHR", "EGKK": "LGW", "EHAM": "AMS", "LFPG": "CDG",
	"EDDF": "FRA", "EDDM": "MUC", "LEMD": "MAD", "LEBL": "BCN",
	"LIRF": "FCO", "LIMC": "MXP", "LSZH": "ZRH", "EKCH": "CPH",
	"ESSA": "ARN", "ENGM": "OSL", "EIDW": "DUB", "EPWA": "WAW",
	"LOWW": "VIE", "EBBR": "BRU", "LPPT": "LIS", "LGAV": "ATH",
	"BIKF": "KEF",
	// Asia
	"RJTT": "HND", "RJAA": "NRT", "RKSI": "ICN", "VHHH": "HKG",
	"WSSS": "SIN", "VTBS": "BKK", "RPLL": "MNL", "ZUUU": "CTU",
	"ZBAA": "PEK", "ZSPD": "PVG",
	// Mexico/Caribbean
	"MMUN": "CUN", "MMSD": "SJD", "MMPR": "PVR", "MMCZ": "CZM",
	"MMMX": "MEX", "MMGL": "GDL", "MMMY": "MTY", "MMTL": "TLC",
	"MPTO": "PTY", "MROC": "SJO", "MRLB": "LIR", "MGGT": "GUA",
	"MHLM": "SAP", "MHRO": "RTB", "MSLP": "SAL", "MZBZ": "BZE",
	"MKJP": "KIN", "MKJS": "MBJ", "MWCR": "GCM",
	"TNCM": "SXM", "TNCA": "AUA", "TNCB": "BON", "TNCC": "CUR",
	"TIST": "STT", "TISX": "STX", "TJSJ": "SJU",
	"MYNN": "NAS", "MBPV": "PLS", "MDPC": "PUJ", "MDPP": "POP",
	"MDSD": "SDQ", "MDST": "STI", "TAPA": "ANU", "TBPB": "BGI",
	"TLPL": "SLU", "TKPK": "SKB",
	// South America
	"SBGR": "GRU", "SAEZ": "EZE", "SCEL": "SCL", "SPJC": "LIM",
	// Africa
	"FAOR": "JNB", "FACT": "CPT", "GMMX": "RAK", "GOBD": "DSS",
	"DNMM": "LOS",
	// Middle East
	"LLBG": "TLV", "OMDB": "DXB", "OEJN": "JED",
	// Oceania
	"YSSY": "SYD", "YMML": "MEL", "NZAA": "AKL",
	// Pacific
	"NTAA": "PPT",
	// US smaller airports commonly seen in SEA traffic
	"KABQ": "ABQ", "KAGS": "AGS", "KALB": "ALB", "KAUS": "AUS",
	"KAVL": "AVL", "KBDL": "BDL", "KBHM": "BHM", "KBIL": "BIL",
	"KBIS": "BIS", "KBOI": "BOI", "KBTV": "BTV", "KBTR": "BTR",
	"KBUF": "BUF", "KBZN": "BZN", "KCAE": "CAE", "KCHA": "CHA",
	"KCHS": "CHS", "KCOS": "COS", "KCRW": "CRW", "KDSM": "DSM",
	"KECP": "ECP", "KEYW": "EYW", "KFAR": "FAR", "KFSD": "FSD",
	"KGNV": "GNV", "KGPI": "FCA", "KGPT": "GPT", "KGRR": "GRR",
	"KGSO": "GSO", "KGSP": "GSP", "KHPN": "HPN", "KHRL": "HRL",
	"KHSV": "HSV", "KICT": "ICT", "KIDA": "IDA", "KJAN": "JAN",
	"KLGB": "LGB", "KLEX": "LEX", "KMDT": "MDT", "KMLB": "MLB",
	"KMOB": "MOB", "KMSO": "MSO", "KMYR": "MYR", "KORF": "ORF",
	"KPNS": "PNS", "KPSC": "PSC", "KPWM": "PWM", "KROA": "ROA",
	"KROC": "ROC", "KSAV": "SAV", "KSRQ": "SRQ", "KSYR": "SYR",
	"KTLH": "TLH", "KTUL": "TUL", "KTVC": "TVC", "KTYS": "TYS",
	"KVPS": "VPS", "KATW": "ATW", "KEUG": "EUG",
	"LIPZ": "VCE", "LIRN": "NAP", "LFMN": "NCE", "EDDL": "DUS",
	"EGPH": "EDI",
}

// ICAOToIATA converts an ICAO airport code to IATA for display.
// If the code is in the map, returns the IATA code.
// If the code starts with 'K' and is 4 chars (US domestic), strips the K.
// Otherwise returns the ICAO code as-is.
func ICAOToIATA(icao string) string {
	if iata, ok := icaoToIATA[icao]; ok {
		return iata
	}
	// US airports: ICAO = "K" + IATA
	if len(icao) == 4 && icao[0] == 'K' {
		return icao[1:]
	}
	return icao
}
