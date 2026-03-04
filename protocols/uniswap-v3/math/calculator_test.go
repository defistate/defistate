package uniswapv3

import (
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =================================================================
// Test Helpers
// =================================================================

// fromString is a helper to create a big.Int from a string for tests.
func fromString(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("failed to set string for big.Int")
	}
	return n
}

func negBigInt(x *big.Int) *big.Int {
	return x.Neg(x)
}

// Raw tick data from a real pool snapshot.
var rawTicks []struct {
	TickIdx      string
	LiquidityNet string
}

var rawTicksJson = `[
      {
        "liquidityGross": "44978760068372456",
        "liquidityNet": "44978760068372456",
        "tickIdx": "-887270"
      },
      {
        "liquidityGross": "23703747677073",
        "liquidityNet": "23703747677073",
        "tickIdx": "-887260"
      },
      {
        "liquidityGross": "398290794261",
        "liquidityNet": "398290794261",
        "tickIdx": "-92110"
      },
      {
        "liquidityGross": "29651014881301328537",
        "liquidityNet": "29651014881301328537",
        "tickIdx": "-76280"
      },
      {
        "liquidityGross": "29651014881301328537",
        "liquidityNet": "-29651014881301328537",
        "tickIdx": "-75980"
      },
      {
        "liquidityGross": "13570724854034",
        "liquidityNet": "13570724854034",
        "tickIdx": "0"
      },
      {
        "liquidityGross": "100",
        "liquidityNet": "100",
        "tickIdx": "100"
      },
      {
        "liquidityGross": "100",
        "liquidityNet": "-100",
        "tickIdx": "110"
      },
      {
        "liquidityGross": "13570724854034",
        "liquidityNet": "-13570724854034",
        "tickIdx": "100000"
      },
      {
        "liquidityGross": "2266789968",
        "liquidityNet": "2266789968",
        "tickIdx": "108340"
      },
      {
        "liquidityGross": "2273693713",
        "liquidityNet": "2273693713",
        "tickIdx": "108360"
      },
      {
        "liquidityGross": "44739669244",
        "liquidityNet": "44739669244",
        "tickIdx": "108390"
      },
      {
        "liquidityGross": "144810683332502",
        "liquidityNet": "144810683332502",
        "tickIdx": "115120"
      },
      {
        "liquidityGross": "24308826080",
        "liquidityNet": "24308826080",
        "tickIdx": "115140"
      },
      {
        "liquidityGross": "2344264358099",
        "liquidityNet": "2344264358099",
        "tickIdx": "138160"
      },
      {
        "liquidityGross": "9403707956281",
        "liquidityNet": "9403707956281",
        "tickIdx": "149150"
      },
      {
        "liquidityGross": "574355480",
        "liquidityNet": "574355480",
        "tickIdx": "161190"
      },
      {
        "liquidityGross": "607207624019674",
        "liquidityNet": "607207624019674",
        "tickIdx": "166300"
      },
      {
        "liquidityGross": "498531057962305",
        "liquidityNet": "498531057962305",
        "tickIdx": "175170"
      },
      {
        "liquidityGross": "1142415145764782",
        "liquidityNet": "1142415145764782",
        "tickIdx": "177250"
      },
      {
        "liquidityGross": "2588778177052193",
        "liquidityNet": "2588778177052193",
        "tickIdx": "177280"
      },
      {
        "liquidityGross": "68579783371570",
        "liquidityNet": "68579783371570",
        "tickIdx": "178340"
      },
      {
        "liquidityGross": "655281438629235",
        "liquidityNet": "655281438629235",
        "tickIdx": "180160"
      },
      {
        "liquidityGross": "419968624358816",
        "liquidityNet": "419968624358816",
        "tickIdx": "180180"
      },
      {
        "liquidityGross": "227351883552117",
        "liquidityNet": "227351883552117",
        "tickIdx": "180440"
      },
      {
        "liquidityGross": "111626068365034",
        "liquidityNet": "111626068365034",
        "tickIdx": "180650"
      },
      {
        "liquidityGross": "88680311208",
        "liquidityNet": "88680311208",
        "tickIdx": "181260"
      },
      {
        "liquidityGross": "105220981852142",
        "liquidityNet": "105220981852142",
        "tickIdx": "181590"
      },
      {
        "liquidityGross": "1131670065466301",
        "liquidityNet": "1131670065466301",
        "tickIdx": "182200"
      },
      {
        "liquidityGross": "3451150785812214",
        "liquidityNet": "3451150785812214",
        "tickIdx": "182390"
      },
      {
        "liquidityGross": "4740323423268196",
        "liquidityNet": "4740323423268196",
        "tickIdx": "182560"
      },
      {
        "liquidityGross": "1084560033827482",
        "liquidityNet": "1084560033827482",
        "tickIdx": "182770"
      },
      {
        "liquidityGross": "4740323423268196",
        "liquidityNet": "-4740323423268196",
        "tickIdx": "183080"
      },
      {
        "liquidityGross": "1104057347094",
        "liquidityNet": "1104057347094",
        "tickIdx": "183160"
      },
      {
        "liquidityGross": "14920025509875",
        "liquidityNet": "14920025509875",
        "tickIdx": "183260"
      },
      {
        "liquidityGross": "151041316619269",
        "liquidityNet": "151041316619269",
        "tickIdx": "183920"
      },
      {
        "liquidityGross": "17409290878",
        "liquidityNet": "17409290878",
        "tickIdx": "184200"
      },
      {
        "liquidityGross": "1691342475368310",
        "liquidityNet": "1691342475368310",
        "tickIdx": "184210"
      },
      {
        "liquidityGross": "5525404060502551",
        "liquidityNet": "5523195945808363",
        "tickIdx": "184220"
      },
      {
        "liquidityGross": "3397635186717387",
        "liquidityNet": "-3397635186717387",
        "tickIdx": "184230"
      },
      {
        "liquidityGross": "477197831652989",
        "liquidityNet": "477197831652989",
        "tickIdx": "185050"
      },
      {
        "liquidityGross": "1049687190863",
        "liquidityNet": "1049687190863",
        "tickIdx": "185210"
      },
      {
        "liquidityGross": "15469101894419",
        "liquidityNet": "15469101894419",
        "tickIdx": "185240"
      },
      {
        "liquidityGross": "4745503553708786",
        "liquidityNet": "4745503553708786",
        "tickIdx": "185260"
      },
      {
        "liquidityGross": "404583613408945",
        "liquidityNet": "404583613408945",
        "tickIdx": "185270"
      },
      {
        "liquidityGross": "297728093",
        "liquidityNet": "297728093",
        "tickIdx": "185840"
      },
      {
        "liquidityGross": "16737492612081",
        "liquidityNet": "16737492612081",
        "tickIdx": "186060"
      },
      {
        "liquidityGross": "8039254639562",
        "liquidityNet": "8039254639562",
        "tickIdx": "186320"
      },
      {
        "liquidityGross": "228699723058089",
        "liquidityNet": "228699723058089",
        "tickIdx": "186420"
      },
      {
        "liquidityGross": "261844689822052",
        "liquidityNet": "261844689822052",
        "tickIdx": "186430"
      },
      {
        "liquidityGross": "244135744116298",
        "liquidityNet": "244135148660112",
        "tickIdx": "186450"
      },
      {
        "liquidityGross": "416983925278171",
        "liquidityNet": "416983925278171",
        "tickIdx": "186460"
      },
      {
        "liquidityGross": "246350386196077",
        "liquidityNet": "246350386196077",
        "tickIdx": "187090"
      },
      {
        "liquidityGross": "98726939861609",
        "liquidityNet": "98726939861609",
        "tickIdx": "187170"
      },
      {
        "liquidityGross": "11661475258274",
        "liquidityNet": "11661475258274",
        "tickIdx": "187440"
      },
      {
        "liquidityGross": "454493636691910",
        "liquidityNet": "454493636691910",
        "tickIdx": "187500"
      },
      {
        "liquidityGross": "2670481682301302",
        "liquidityNet": "-411616576700922",
        "tickIdx": "187780"
      },
      {
        "liquidityGross": "54743213803487",
        "liquidityNet": "54743213803487",
        "tickIdx": "187940"
      },
      {
        "liquidityGross": "184855906959178362",
        "liquidityNet": "184855906959178362",
        "tickIdx": "188130"
      },
      {
        "liquidityGross": "5020580219556",
        "liquidityNet": "5020580219556",
        "tickIdx": "188440"
      },
      {
        "liquidityGross": "10013802424018",
        "liquidityNet": "10013802424018",
        "tickIdx": "188450"
      },
      {
        "liquidityGross": "132473552239106",
        "liquidityNet": "132473552239106",
        "tickIdx": "188520"
      },
      {
        "liquidityGross": "224653000969518",
        "liquidityNet": "224653000969518",
        "tickIdx": "188780"
      },
      {
        "liquidityGross": "6391022318941913",
        "liquidityNet": "6391022318941913",
        "tickIdx": "188990"
      },
      {
        "liquidityGross": "431254538102",
        "liquidityNet": "431254538102",
        "tickIdx": "189020"
      },
      {
        "liquidityGross": "4459378691580",
        "liquidityNet": "4459378691580",
        "tickIdx": "189040"
      },
      {
        "liquidityGross": "1491132445622",
        "liquidityNet": "1491132445622",
        "tickIdx": "189130"
      },
      {
        "liquidityGross": "107388554969",
        "liquidityNet": "107388554969",
        "tickIdx": "189210"
      },
      {
        "liquidityGross": "116526800732",
        "liquidityNet": "116526800732",
        "tickIdx": "189230"
      },
      {
        "liquidityGross": "21054643810291374",
        "liquidityNet": "21054643810291374",
        "tickIdx": "189320"
      },
      {
        "liquidityGross": "13452407513465956",
        "liquidityNet": "13452407513465956",
        "tickIdx": "189340"
      },
      {
        "liquidityGross": "110366245499632",
        "liquidityNet": "110366245499632",
        "tickIdx": "189410"
      },
      {
        "liquidityGross": "2659554920775",
        "liquidityNet": "2659554920775",
        "tickIdx": "189460"
      },
      {
        "liquidityGross": "453600213217",
        "liquidityNet": "453600213217",
        "tickIdx": "189490"
      },
      {
        "liquidityGross": "4355219688957",
        "liquidityNet": "4355219688957",
        "tickIdx": "189520"
      },
      {
        "liquidityGross": "1034480184182597",
        "liquidityNet": "1034480184182597",
        "tickIdx": "189880"
      },
      {
        "liquidityGross": "42201964560379",
        "liquidityNet": "42201964560379",
        "tickIdx": "189920"
      },
      {
        "liquidityGross": "941735738993078",
        "liquidityNet": "941735738993078",
        "tickIdx": "189990"
      },
      {
        "liquidityGross": "11591027949234734",
        "liquidityNet": "11591027949234734",
        "tickIdx": "190080"
      },
      {
        "liquidityGross": "624068026011",
        "liquidityNet": "624068026011",
        "tickIdx": "190100"
      },
      {
        "liquidityGross": "3077564238585",
        "liquidityNet": "3077564238585",
        "tickIdx": "190130"
      },
      {
        "liquidityGross": "2082544348785232",
        "liquidityNet": "2082544348785232",
        "tickIdx": "190140"
      },
      {
        "liquidityGross": "13957834346797159",
        "liquidityNet": "13957834346797159",
        "tickIdx": "190170"
      },
      {
        "liquidityGross": "15637594630692703",
        "liquidityNet": "15637594630692703",
        "tickIdx": "190190"
      },
      {
        "liquidityGross": "1375109716821800",
        "liquidityNet": "1375109716821800",
        "tickIdx": "190220"
      },
      {
        "liquidityGross": "408198383987653",
        "liquidityNet": "408198383987653",
        "tickIdx": "190380"
      },
      {
        "liquidityGross": "57016310416248",
        "liquidityNet": "57016310416248",
        "tickIdx": "190470"
      },
      {
        "liquidityGross": "14327762966156",
        "liquidityNet": "14327762966156",
        "tickIdx": "190500"
      },
      {
        "liquidityGross": "30520418856337",
        "liquidityNet": "30520418856337",
        "tickIdx": "190580"
      },
      {
        "liquidityGross": "9992614566589614",
        "liquidityNet": "9992614566589614",
        "tickIdx": "190640"
      },
      {
        "liquidityGross": "68758133535664",
        "liquidityNet": "68758133535664",
        "tickIdx": "190670"
      },
      {
        "liquidityGross": "71510532495962",
        "liquidityNet": "71510532495962",
        "tickIdx": "190720"
      },
      {
        "liquidityGross": "142660157354006",
        "liquidityNet": "142660157354006",
        "tickIdx": "190730"
      },
      {
        "liquidityGross": "4499788938440932",
        "liquidityNet": "4499788938440932",
        "tickIdx": "190780"
      },
      {
        "liquidityGross": "224082011199",
        "liquidityNet": "224082011199",
        "tickIdx": "190800"
      },
      {
        "liquidityGross": "88934515412244",
        "liquidityNet": "88934515412244",
        "tickIdx": "190890"
      },
      {
        "liquidityGross": "216130743921190",
        "liquidityNet": "216130743921190",
        "tickIdx": "190920"
      },
      {
        "liquidityGross": "467104856038985",
        "liquidityNet": "467104856038985",
        "tickIdx": "190950"
      },
      {
        "liquidityGross": "2454142663090112",
        "liquidityNet": "2454142663090112",
        "tickIdx": "191020"
      },
      {
        "liquidityGross": "955418267016008",
        "liquidityNet": "955418267016008",
        "tickIdx": "191050"
      },
      {
        "liquidityGross": "2537796806805945",
        "liquidityNet": "2537796806805945",
        "tickIdx": "191110"
      },
      {
        "liquidityGross": "27563266076594",
        "liquidityNet": "27563266076594",
        "tickIdx": "191130"
      },
      {
        "liquidityGross": "1147308847885218",
        "liquidityNet": "1147258262744214",
        "tickIdx": "191140"
      },
      {
        "liquidityGross": "63706934514049943",
        "liquidityNet": "63706933092836595",
        "tickIdx": "191150"
      },
      {
        "liquidityGross": "1620100610323942",
        "liquidityNet": "1620100610323942",
        "tickIdx": "191160"
      },
      {
        "liquidityGross": "59062441893549909",
        "liquidityNet": "59062441893549909",
        "tickIdx": "191170"
      },
      {
        "liquidityGross": "1050191227952063",
        "liquidityNet": "1050191227952063",
        "tickIdx": "191200"
      },
      {
        "liquidityGross": "15210320549012533",
        "liquidityNet": "15210320549012533",
        "tickIdx": "191210"
      },
      {
        "liquidityGross": "100397645365749",
        "liquidityNet": "100397645365749",
        "tickIdx": "191250"
      },
      {
        "liquidityGross": "1208304338184526",
        "liquidityNet": "1208304338184526",
        "tickIdx": "191270"
      },
      {
        "liquidityGross": "4401650642802915",
        "liquidityNet": "4401650642802915",
        "tickIdx": "191290"
      },
      {
        "liquidityGross": "47944910966622481",
        "liquidityNet": "47944910966622481",
        "tickIdx": "191340"
      },
      {
        "liquidityGross": "3150344442876953",
        "liquidityNet": "3150344442876953",
        "tickIdx": "191350"
      },
      {
        "liquidityGross": "5188820077378081",
        "liquidityNet": "5188820077378081",
        "tickIdx": "191360"
      },
      {
        "liquidityGross": "27338361178403975",
        "liquidityNet": "27338361178403975",
        "tickIdx": "191380"
      },
      {
        "liquidityGross": "6783329557647",
        "liquidityNet": "6783329557647",
        "tickIdx": "191390"
      },
      {
        "liquidityGross": "5039433626820",
        "liquidityNet": "5039433626820",
        "tickIdx": "191400"
      },
      {
        "liquidityGross": "2094699975947",
        "liquidityNet": "2094699975947",
        "tickIdx": "191410"
      },
      {
        "liquidityGross": "359772522343182",
        "liquidityNet": "359772522343182",
        "tickIdx": "191420"
      },
      {
        "liquidityGross": "62625442344491",
        "liquidityNet": "62625442344491",
        "tickIdx": "191450"
      },
      {
        "liquidityGross": "803705306501235",
        "liquidityNet": "803705306501235",
        "tickIdx": "191490"
      },
      {
        "liquidityGross": "17473516102072026",
        "liquidityNet": "17473516102072026",
        "tickIdx": "191560"
      },
      {
        "liquidityGross": "748462594481221",
        "liquidityNet": "748462594481221",
        "tickIdx": "191580"
      },
      {
        "liquidityGross": "524896368645537",
        "liquidityNet": "524896368645537",
        "tickIdx": "191600"
      },
      {
        "liquidityGross": "75338968183905158",
        "liquidityNet": "75338968183905158",
        "tickIdx": "191640"
      },
      {
        "liquidityGross": "1566850962828684",
        "liquidityNet": "1566850962828684",
        "tickIdx": "191690"
      },
      {
        "liquidityGross": "1622109959153899",
        "liquidityNet": "1622109959153899",
        "tickIdx": "191770"
      },
      {
        "liquidityGross": "16754067954091",
        "liquidityNet": "16754067954091",
        "tickIdx": "191780"
      },
      {
        "liquidityGross": "69660612896438",
        "liquidityNet": "69660612896438",
        "tickIdx": "191820"
      },
      {
        "liquidityGross": "110172609084353",
        "liquidityNet": "110172609084353",
        "tickIdx": "191840"
      },
      {
        "liquidityGross": "17110416892885",
        "liquidityNet": "17110416892885",
        "tickIdx": "191860"
      },
      {
        "liquidityGross": "761140366858189",
        "liquidityNet": "761140366858189",
        "tickIdx": "191890"
      },
      {
        "liquidityGross": "10761483450755904",
        "liquidityNet": "10761483450755904",
        "tickIdx": "191930"
      },
      {
        "liquidityGross": "94566952731702183",
        "liquidityNet": "94566952731702183",
        "tickIdx": "191940"
      },
      {
        "liquidityGross": "73984870976854",
        "liquidityNet": "73984870976854",
        "tickIdx": "191950"
      },
      {
        "liquidityGross": "1212772494312444",
        "liquidityNet": "1212772494312444",
        "tickIdx": "191960"
      },
      {
        "liquidityGross": "10959100062215967",
        "liquidityNet": "10959100062215967",
        "tickIdx": "191980"
      },
      {
        "liquidityGross": "1666345879759411",
        "liquidityNet": "-916999800151269",
        "tickIdx": "192010"
      },
      {
        "liquidityGross": "162007238007207",
        "liquidityNet": "-132066617116831",
        "tickIdx": "192040"
      },
      {
        "liquidityGross": "110172609084353",
        "liquidityNet": "-110172609084353",
        "tickIdx": "192060"
      },
      {
        "liquidityGross": "1142710830330571",
        "liquidityNet": "1142710830330571",
        "tickIdx": "192070"
      },
      {
        "liquidityGross": "3635690949857831",
        "liquidityNet": "3635690949857831",
        "tickIdx": "192090"
      },
      {
        "liquidityGross": "6795425165288",
        "liquidityNet": "6795425165288",
        "tickIdx": "192110"
      },
      {
        "liquidityGross": "30070437780982754",
        "liquidityNet": "30070437780982754",
        "tickIdx": "192130"
      },
      {
        "liquidityGross": "12671773620243808",
        "liquidityNet": "12671773620243808",
        "tickIdx": "192160"
      },
      {
        "liquidityGross": "360377541178992221",
        "liquidityNet": "360377541178992221",
        "tickIdx": "192200"
      },
      {
        "liquidityGross": "1094500921135",
        "liquidityNet": "1094500921135",
        "tickIdx": "192220"
      },
      {
        "liquidityGross": "9981145729025",
        "liquidityNet": "9981145729025",
        "tickIdx": "192230"
      },
      {
        "liquidityGross": "66336695746741",
        "liquidityNet": "66336695746741",
        "tickIdx": "192240"
      },
      {
        "liquidityGross": "5851499863231400",
        "liquidityNet": "5851499863231400",
        "tickIdx": "192250"
      },
      {
        "liquidityGross": "525812305488578",
        "liquidityNet": "525812305488578",
        "tickIdx": "192270"
      },
      {
        "liquidityGross": "97041609388172",
        "liquidityNet": "97041609388172",
        "tickIdx": "192300"
      },
      {
        "liquidityGross": "68558787967364",
        "liquidityNet": "68558787967364",
        "tickIdx": "192310"
      },
      {
        "liquidityGross": "39372497843493362",
        "liquidityNet": "39372497843493362",
        "tickIdx": "192320"
      },
      {
        "liquidityGross": "595664739440891",
        "liquidityNet": "595664739440891",
        "tickIdx": "192330"
      },
      {
        "liquidityGross": "13506826513573",
        "liquidityNet": "13506826513573",
        "tickIdx": "192340"
      },
      {
        "liquidityGross": "10859893043296587",
        "liquidityNet": "-10135076653441127",
        "tickIdx": "192430"
      },
      {
        "liquidityGross": "5028056873592845",
        "liquidityNet": "5028054492639427",
        "tickIdx": "192540"
      },
      {
        "liquidityGross": "313389198396341",
        "liquidityNet": "313389198396341",
        "tickIdx": "192550"
      },
      {
        "liquidityGross": "852270591555056",
        "liquidityNet": "852270591555056",
        "tickIdx": "192560"
      },
      {
        "liquidityGross": "1101283002618",
        "liquidityNet": "1101283002618",
        "tickIdx": "192570"
      },
      {
        "liquidityGross": "5210098515299959",
        "liquidityNet": "5210098515299959",
        "tickIdx": "192580"
      },
      {
        "liquidityGross": "785183510489161",
        "liquidityNet": "785183510489161",
        "tickIdx": "192590"
      },
      {
        "liquidityGross": "3790906020488205",
        "liquidityNet": "-3790906020488205",
        "tickIdx": "192600"
      },
      {
        "liquidityGross": "64130601516847",
        "liquidityNet": "64130601516847",
        "tickIdx": "192630"
      },
      {
        "liquidityGross": "533656801435810",
        "liquidityNet": "533656801435810",
        "tickIdx": "192650"
      },
      {
        "liquidityGross": "22315429910804164",
        "liquidityNet": "22315429910804164",
        "tickIdx": "192660"
      },
      {
        "liquidityGross": "6724265650213026",
        "liquidityNet": "6724265650213026",
        "tickIdx": "192690"
      },
      {
        "liquidityGross": "29001865383163",
        "liquidityNet": "29001865383163",
        "tickIdx": "192700"
      },
      {
        "liquidityGross": "9333788502352",
        "liquidityNet": "9333788502352",
        "tickIdx": "192720"
      },
      {
        "liquidityGross": "3362222189939151",
        "liquidityNet": "3362222189939151",
        "tickIdx": "192730"
      },
      {
        "liquidityGross": "319612842172412",
        "liquidityNet": "319612842172412",
        "tickIdx": "192740"
      },
      {
        "liquidityGross": "21037253742886",
        "liquidityNet": "21037253742886",
        "tickIdx": "192750"
      },
      {
        "liquidityGross": "1104912081308990",
        "liquidityNet": "1104912081308990",
        "tickIdx": "192760"
      },
      {
        "liquidityGross": "16790385442928",
        "liquidityNet": "16790385442928",
        "tickIdx": "192770"
      },
      {
        "liquidityGross": "502989459558382",
        "liquidityNet": "502989459558382",
        "tickIdx": "192780"
      },
      {
        "liquidityGross": "939053049297",
        "liquidityNet": "939053049297",
        "tickIdx": "192790"
      },
      {
        "liquidityGross": "334384264738564",
        "liquidityNet": "334384264738564",
        "tickIdx": "192800"
      },
      {
        "liquidityGross": "577460946635631",
        "liquidityNet": "577460946635631",
        "tickIdx": "192810"
      },
      {
        "liquidityGross": "992012084007272",
        "liquidityNet": "992012084007272",
        "tickIdx": "192820"
      },
      {
        "liquidityGross": "18009985194391259",
        "liquidityNet": "18009985194391259",
        "tickIdx": "192850"
      },
      {
        "liquidityGross": "144702553708173",
        "liquidityNet": "144702553708173",
        "tickIdx": "192860"
      },
      {
        "liquidityGross": "3316481380792550",
        "liquidityNet": "3316481380792550",
        "tickIdx": "192880"
      },
      {
        "liquidityGross": "362828155770846340",
        "liquidityNet": "362828155770846340",
        "tickIdx": "192890"
      },
      {
        "liquidityGross": "1190338611698300",
        "liquidityNet": "1190338611698300",
        "tickIdx": "192920"
      },
      {
        "liquidityGross": "4147729363982767",
        "liquidityNet": "4147729363982767",
        "tickIdx": "192930"
      },
      {
        "liquidityGross": "633385112841461",
        "liquidityNet": "633385112841461",
        "tickIdx": "192940"
      },
      {
        "liquidityGross": "243555239068975",
        "liquidityNet": "243555239068975",
        "tickIdx": "192950"
      },
      {
        "liquidityGross": "792418458417283",
        "liquidityNet": "792418458417283",
        "tickIdx": "192960"
      },
      {
        "liquidityGross": "11747497668180204",
        "liquidityNet": "11747497668180204",
        "tickIdx": "192980"
      },
      {
        "liquidityGross": "5582703905810",
        "liquidityNet": "5582703905810",
        "tickIdx": "193000"
      },
      {
        "liquidityGross": "2899295615375",
        "liquidityNet": "2618642453423",
        "tickIdx": "193010"
      },
      {
        "liquidityGross": "52855866853167758",
        "liquidityNet": "52855866853167758",
        "tickIdx": "193020"
      },
      {
        "liquidityGross": "1592364244729474",
        "liquidityNet": "1592364244729474",
        "tickIdx": "193030"
      },
      {
        "liquidityGross": "530903589704407",
        "liquidityNet": "530903589704407",
        "tickIdx": "193040"
      },
      {
        "liquidityGross": "239551431053309",
        "liquidityNet": "239551431053309",
        "tickIdx": "193050"
      },
      {
        "liquidityGross": "5834532636257724",
        "liquidityNet": "5834532636257724",
        "tickIdx": "193070"
      },
      {
        "liquidityGross": "9415524806683644",
        "liquidityNet": "9415524806683644",
        "tickIdx": "193080"
      },
      {
        "liquidityGross": "159182955381611",
        "liquidityNet": "159182955381611",
        "tickIdx": "193110"
      },
      {
        "liquidityGross": "17582514774929",
        "liquidityNet": "17582514774929",
        "tickIdx": "193120"
      },
      {
        "liquidityGross": "23802028003246999",
        "liquidityNet": "23802028003246999",
        "tickIdx": "193130"
      },
      {
        "liquidityGross": "1268220779490710",
        "liquidityNet": "1268220779490710",
        "tickIdx": "193150"
      },
      {
        "liquidityGross": "428593410094638",
        "liquidityNet": "428396012173128",
        "tickIdx": "193160"
      },
      {
        "liquidityGross": "6164324568043073",
        "liquidityNet": "6164324568043073",
        "tickIdx": "193170"
      },
      {
        "liquidityGross": "7270571658410040",
        "liquidityNet": "-3578149220718032",
        "tickIdx": "193180"
      },
      {
        "liquidityGross": "252640582079213",
        "liquidityNet": "252640582079213",
        "tickIdx": "193190"
      },
      {
        "liquidityGross": "8742335331739934",
        "liquidityNet": "8742335331739934",
        "tickIdx": "193200"
      },
      {
        "liquidityGross": "21429840127304878",
        "liquidityNet": "21429840127304878",
        "tickIdx": "193210"
      },
      {
        "liquidityGross": "24863651835896306",
        "liquidityNet": "21713419551694964",
        "tickIdx": "193220"
      },
      {
        "liquidityGross": "505681290820892",
        "liquidityNet": "505681290820892",
        "tickIdx": "193240"
      },
      {
        "liquidityGross": "2896529144969",
        "liquidityNet": "2896529144969",
        "tickIdx": "193250"
      },
      {
        "liquidityGross": "71128033132486",
        "liquidityNet": "71128033132486",
        "tickIdx": "193260"
      },
      {
        "liquidityGross": "417405379582506267",
        "liquidityNet": "417405379582506267",
        "tickIdx": "193270"
      },
      {
        "liquidityGross": "16386661756838",
        "liquidityNet": "16386661756838",
        "tickIdx": "193280"
      },
      {
        "liquidityGross": "147629331540997",
        "liquidityNet": "147433212481233",
        "tickIdx": "193290"
      },
      {
        "liquidityGross": "10423298847805999",
        "liquidityNet": "10423298847805999",
        "tickIdx": "193300"
      },
      {
        "liquidityGross": "92922799549920",
        "liquidityNet": "92922799549920",
        "tickIdx": "193310"
      },
      {
        "liquidityGross": "11299792213255",
        "liquidityNet": "11299792213255",
        "tickIdx": "193320"
      },
      {
        "liquidityGross": "481987597446560",
        "liquidityNet": "481987597446560",
        "tickIdx": "193330"
      },
      {
        "liquidityGross": "23156212079566447",
        "liquidityNet": "23145770992708597",
        "tickIdx": "193340"
      },
      {
        "liquidityGross": "52585024153162",
        "liquidityNet": "52585024153162",
        "tickIdx": "193350"
      },
      {
        "liquidityGross": "10490869572965500",
        "liquidityNet": "10490869572965500",
        "tickIdx": "193360"
      },
      {
        "liquidityGross": "10787423134689825",
        "liquidityNet": "1296405839322119",
        "tickIdx": "193370"
      },
      {
        "liquidityGross": "70310058764139430",
        "liquidityNet": "64914036868626052",
        "tickIdx": "193380"
      },
      {
        "liquidityGross": "13783786281720",
        "liquidityNet": "13783786281720",
        "tickIdx": "193390"
      },
      {
        "liquidityGross": "1797101466719256",
        "liquidityNet": "1797101466719256",
        "tickIdx": "193400"
      },
      {
        "liquidityGross": "32379831000142809",
        "liquidityNet": "32379831000142809",
        "tickIdx": "193410"
      },
      {
        "liquidityGross": "951754252239215",
        "liquidityNet": "-199628890502649",
        "tickIdx": "193420"
      },
      {
        "liquidityGross": "2185595125629027647",
        "liquidityNet": "2067470241841927829",
        "tickIdx": "193430"
      },
      {
        "liquidityGross": "4359958076351360",
        "liquidityNet": "4359958076351360",
        "tickIdx": "193440"
      },
      {
        "liquidityGross": "181485852009596",
        "liquidityNet": "181485852009596",
        "tickIdx": "193450"
      },
      {
        "liquidityGross": "712799131895416801",
        "liquidityNet": "-122011627269595733",
        "tickIdx": "193470"
      },
      {
        "liquidityGross": "535474783084873",
        "liquidityNet": "535474783084873",
        "tickIdx": "193480"
      },
      {
        "liquidityGross": "843312834670442",
        "liquidityNet": "843312834670442",
        "tickIdx": "193500"
      },
      {
        "liquidityGross": "42955992000798087",
        "liquidityNet": "42955992000798087",
        "tickIdx": "193510"
      },
      {
        "liquidityGross": "157444189101167",
        "liquidityNet": "-157444189101167",
        "tickIdx": "193530"
      },
      {
        "liquidityGross": "47606322110504996",
        "liquidityNet": "47606128524366704",
        "tickIdx": "193550"
      },
      {
        "liquidityGross": "121196699469522",
        "liquidityNet": "-51191301283478",
        "tickIdx": "193560"
      },
      {
        "liquidityGross": "417345483866658",
        "liquidityNet": "417345483866658",
        "tickIdx": "193570"
      },
      {
        "liquidityGross": "407959931472533",
        "liquidityNet": "49519250864937",
        "tickIdx": "193580"
      },
      {
        "liquidityGross": "242747533382",
        "liquidityNet": "242747533382",
        "tickIdx": "193590"
      },
      {
        "liquidityGross": "420415578427063",
        "liquidityNet": "-414275389306253",
        "tickIdx": "193600"
      },
      {
        "liquidityGross": "242747533382",
        "liquidityNet": "-242747533382",
        "tickIdx": "193610"
      },
      {
        "liquidityGross": "4859155000976252",
        "liquidityNet": "4859155000976252",
        "tickIdx": "193620"
      },
      {
        "liquidityGross": "317413508691314756",
        "liquidityNet": "-1628492552956600",
        "tickIdx": "193630"
      },
      {
        "liquidityGross": "136596602628109",
        "liquidityNet": "136596602628109",
        "tickIdx": "193640"
      },
      {
        "liquidityGross": "2394724731721493",
        "liquidityNet": "2394724731721493",
        "tickIdx": "193650"
      },
      {
        "liquidityGross": "4868691739636577",
        "liquidityNet": "-4850758465151173",
        "tickIdx": "193660"
      },
      {
        "liquidityGross": "6452158211314424",
        "liquidityNet": "1662708747871438",
        "tickIdx": "193670"
      },
      {
        "liquidityGross": "22667950560196",
        "liquidityNet": "22667950560196",
        "tickIdx": "193690"
      },
      {
        "liquidityGross": "344791059519915",
        "liquidityNet": "344791059519915",
        "tickIdx": "193700"
      },
      {
        "liquidityGross": "125610818733604",
        "liquidityNet": "125610818733604",
        "tickIdx": "193720"
      },
      {
        "liquidityGross": "632590114305662682",
        "liquidityNet": "80687310874539236",
        "tickIdx": "193730"
      },
      {
        "liquidityGross": "226186835976284",
        "liquidityNet": "226186835976284",
        "tickIdx": "193740"
      },
      {
        "liquidityGross": "9725248577948995",
        "liquidityNet": "9676773964672429",
        "tickIdx": "193760"
      },
      {
        "liquidityGross": "3566136291796822",
        "liquidityNet": "3536195670906446",
        "tickIdx": "193770"
      },
      {
        "liquidityGross": "31261767755462026",
        "liquidityNet": "31261767755462026",
        "tickIdx": "193790"
      },
      {
        "liquidityGross": "49812768593933178",
        "liquidityNet": "49806628404812368",
        "tickIdx": "193800"
      },
      {
        "liquidityGross": "75757380418552555",
        "liquidityNet": "-75677653807476697",
        "tickIdx": "193810"
      },
      {
        "liquidityGross": "1042789587533",
        "liquidityNet": "1042789587533",
        "tickIdx": "193820"
      },
      {
        "liquidityGross": "87376519580629",
        "liquidityNet": "87376519580629",
        "tickIdx": "193830"
      },
      {
        "liquidityGross": "867536577382586",
        "liquidityNet": "867447659653344",
        "tickIdx": "193840"
      },
      {
        "liquidityGross": "2950851383647149",
        "liquidityNet": "-2950851383647149",
        "tickIdx": "193850"
      },
      {
        "liquidityGross": "40480786716227",
        "liquidityNet": "40480786716227",
        "tickIdx": "193860"
      },
      {
        "liquidityGross": "9310357830628273",
        "liquidityNet": "9310357830628273",
        "tickIdx": "193870"
      },
      {
        "liquidityGross": "17878397051958011",
        "liquidityNet": "17446010840240743",
        "tickIdx": "193890"
      },
      {
        "liquidityGross": "2293631420519",
        "liquidityNet": "2293631420519",
        "tickIdx": "193900"
      },
      {
        "liquidityGross": "9074419579393601",
        "liquidityNet": "9074419579393601",
        "tickIdx": "193910"
      },
      {
        "liquidityGross": "26576634957459596",
        "liquidityNet": "19106053362174780",
        "tickIdx": "193920"
      },
      {
        "liquidityGross": "361191804280492806",
        "liquidityNet": "-352068727411541916",
        "tickIdx": "193930"
      },
      {
        "liquidityGross": "23209715064024249",
        "liquidityNet": "-23209715064024249",
        "tickIdx": "193940"
      },
      {
        "liquidityGross": "841184624190776",
        "liquidityNet": "-410332675601246",
        "tickIdx": "193950"
      },
      {
        "liquidityGross": "14565145856286956",
        "liquidityNet": "12762712641222638",
        "tickIdx": "193960"
      },
      {
        "liquidityGross": "4453966892131775",
        "liquidityNet": "-4117009959750143",
        "tickIdx": "193970"
      },
      {
        "liquidityGross": "7357439952526224",
        "liquidityNet": "-6250767265634002",
        "tickIdx": "193980"
      },
      {
        "liquidityGross": "250040995586732",
        "liquidityNet": "-250040995586732",
        "tickIdx": "193990"
      },
      {
        "liquidityGross": "677643596519793",
        "liquidityNet": "-443409637318285",
        "tickIdx": "194000"
      },
      {
        "liquidityGross": "5549535135892810",
        "liquidityNet": "5549535135892810",
        "tickIdx": "194010"
      },
      {
        "liquidityGross": "5401316418748966",
        "liquidityNet": "-5171938342261516",
        "tickIdx": "194020"
      },
      {
        "liquidityGross": "2725854934748987",
        "liquidityNet": "-2691114024433829",
        "tickIdx": "194040"
      },
      {
        "liquidityGross": "145218161656030047",
        "liquidityNet": "145218161656030047",
        "tickIdx": "194050"
      },
      {
        "liquidityGross": "98448842312524",
        "liquidityNet": "98448842312524",
        "tickIdx": "194060"
      },
      {
        "liquidityGross": "2214490701118183",
        "liquidityNet": "2214490701118183",
        "tickIdx": "194080"
      },
      {
        "liquidityGross": "1706418892245099",
        "liquidityNet": "-1091108088802047",
        "tickIdx": "194090"
      },
      {
        "liquidityGross": "2210946742086708",
        "liquidityNet": "2210946742086708",
        "tickIdx": "194100"
      },
      {
        "liquidityGross": "305739459182454",
        "liquidityNet": "-302660204535608",
        "tickIdx": "194110"
      },
      {
        "liquidityGross": "181093154008826",
        "liquidityNet": "181004700396468",
        "tickIdx": "194120"
      },
      {
        "liquidityGross": "193670774978291",
        "liquidityNet": "170314308516753",
        "tickIdx": "194130"
      },
      {
        "liquidityGross": "6746441018719522",
        "liquidityNet": "6746441018719522",
        "tickIdx": "194150"
      },
      {
        "liquidityGross": "121710161972821833",
        "liquidityNet": "-102339681901280997",
        "tickIdx": "194160"
      },
      {
        "liquidityGross": "21872094733693698",
        "liquidityNet": "21872094733693698",
        "tickIdx": "194170"
      },
      {
        "liquidityGross": "127333777645379",
        "liquidityNet": "127333777645379",
        "tickIdx": "194180"
      },
      {
        "liquidityGross": "1561086980580963",
        "liquidityNet": "1372383597911125",
        "tickIdx": "194190"
      },
      {
        "liquidityGross": "249822638772157",
        "liquidityNet": "-7058490999421",
        "tickIdx": "194200"
      },
      {
        "liquidityGross": "207033217830177",
        "liquidityNet": "-35730929942559",
        "tickIdx": "194210"
      },
      {
        "liquidityGross": "49023756978848650",
        "liquidityNet": "-48772735110768260",
        "tickIdx": "194220"
      },
      {
        "liquidityGross": "231812215555162",
        "liquidityNet": "227114650310992",
        "tickIdx": "194230"
      },
      {
        "liquidityGross": "22743496170389",
        "liquidityNet": "22385491048117",
        "tickIdx": "194250"
      },
      {
        "liquidityGross": "86424360320",
        "liquidityNet": "86424360320",
        "tickIdx": "194260"
      },
      {
        "liquidityGross": "28035277881941",
        "liquidityNet": "28035277881941",
        "tickIdx": "194270"
      },
      {
        "liquidityGross": "37434213071258",
        "liquidityNet": "37434213071258",
        "tickIdx": "194290"
      },
      {
        "liquidityGross": "2127972363979292070",
        "liquidityNet": "-2126758066958248254",
        "tickIdx": "194300"
      },
      {
        "liquidityGross": "8802293757967902",
        "liquidityNet": "-8802293757967902",
        "tickIdx": "194320"
      },
      {
        "liquidityGross": "31897551758233076",
        "liquidityNet": "7642153822967488",
        "tickIdx": "194330"
      },
      {
        "liquidityGross": "22699931112537341",
        "liquidityNet": "22060705428192517",
        "tickIdx": "194340"
      },
      {
        "liquidityGross": "4503319968258673",
        "liquidityNet": "-4496257908623191",
        "tickIdx": "194350"
      },
      {
        "liquidityGross": "10557881185873389",
        "liquidityNet": "-9427347947305839",
        "tickIdx": "194360"
      },
      {
        "liquidityGross": "16450924918597403",
        "liquidityNet": "-16253233826101765",
        "tickIdx": "194370"
      },
      {
        "liquidityGross": "2309533305757",
        "liquidityNet": "-2112911638423",
        "tickIdx": "194380"
      },
      {
        "liquidityGross": "328470755555",
        "liquidityNet": "-328470755555",
        "tickIdx": "194410"
      },
      {
        "liquidityGross": "1058737597323737",
        "liquidityNet": "1058737597323737",
        "tickIdx": "194420"
      },
      {
        "liquidityGross": "295065388302142807",
        "liquidityNet": "-283560318447319017",
        "tickIdx": "194430"
      },
      {
        "liquidityGross": "797943156680418",
        "liquidityNet": "797943156680418",
        "tickIdx": "194440"
      },
      {
        "liquidityGross": "7725206018967450",
        "liquidityNet": "7725206018967450",
        "tickIdx": "194460"
      },
      {
        "liquidityGross": "6881699369420418",
        "liquidityNet": "-6881699369420418",
        "tickIdx": "194470"
      },
      {
        "liquidityGross": "109307854712499",
        "liquidityNet": "-109307854712499",
        "tickIdx": "194480"
      },
      {
        "liquidityGross": "11467664755379124",
        "liquidityNet": "-8540521363237642",
        "tickIdx": "194510"
      },
      {
        "liquidityGross": "721985335123383",
        "liquidityNet": "721985335123383",
        "tickIdx": "194520"
      },
      {
        "liquidityGross": "29111337789713",
        "liquidityNet": "-29111337789713",
        "tickIdx": "194540"
      },
      {
        "liquidityGross": "145192447673490260",
        "liquidityNet": "-145190552984840610",
        "tickIdx": "194550"
      },
      {
        "liquidityGross": "8276668785909773",
        "liquidityNet": "6570232914150011",
        "tickIdx": "194580"
      },
      {
        "liquidityGross": "410217889251662",
        "liquidityNet": "410217889251662",
        "tickIdx": "194590"
      },
      {
        "liquidityGross": "1868877087942040",
        "liquidityNet": "377986830376374",
        "tickIdx": "194600"
      },
      {
        "liquidityGross": "5967819685",
        "liquidityNet": "5967819685",
        "tickIdx": "194610"
      },
      {
        "liquidityGross": "731786621768846",
        "liquidityNet": "731786621768846",
        "tickIdx": "194620"
      },
      {
        "liquidityGross": "1121321520802207",
        "liquidityNet": "-1121321520802207",
        "tickIdx": "194630"
      },
      {
        "liquidityGross": "1398093104964209",
        "liquidityNet": "1079727194200987",
        "tickIdx": "194640"
      },
      {
        "liquidityGross": "148711110339063547",
        "liquidityNet": "148711110339063547",
        "tickIdx": "194650"
      },
      {
        "liquidityGross": "976099557108",
        "liquidityNet": "976099557108",
        "tickIdx": "194660"
      },
      {
        "liquidityGross": "106775541336400",
        "liquidityNet": "106775541336400",
        "tickIdx": "194670"
      },
      {
        "liquidityGross": "152716660181441580",
        "liquidityNet": "-144705560496685514",
        "tickIdx": "194680"
      },
      {
        "liquidityGross": "3379113631709",
        "liquidityNet": "-3375735340417",
        "tickIdx": "194690"
      },
      {
        "liquidityGross": "1231989327617959",
        "liquidityNet": "1224867245277571",
        "tickIdx": "194700"
      },
      {
        "liquidityGross": "145983268278519745",
        "liquidityNet": "42806525257086003",
        "tickIdx": "194710"
      },
      {
        "liquidityGross": "9411837024782150",
        "liquidityNet": "-9322664585430136",
        "tickIdx": "194720"
      },
      {
        "liquidityGross": "21262317876105093",
        "liquidityNet": "10386192166502115",
        "tickIdx": "194730"
      },
      {
        "liquidityGross": "1436303551823626",
        "liquidityNet": "1139674486633280",
        "tickIdx": "194740"
      },
      {
        "liquidityGross": "17233111237378877",
        "liquidityNet": "-17233111237378877",
        "tickIdx": "194750"
      },
      {
        "liquidityGross": "3495033047444402",
        "liquidityNet": "3482793727126212",
        "tickIdx": "194760"
      },
      {
        "liquidityGross": "42087797692117418",
        "liquidityNet": "-42087797692117418",
        "tickIdx": "194770"
      },
      {
        "liquidityGross": "3734920884483280",
        "liquidityNet": "-3538961023559730",
        "tickIdx": "194780"
      },
      {
        "liquidityGross": "17615571132218677",
        "liquidityNet": "17615278098816611",
        "tickIdx": "194790"
      },
      {
        "liquidityGross": "7326149028989228",
        "liquidityNet": "-7301626851550666",
        "tickIdx": "194800"
      },
      {
        "liquidityGross": "1198884537287",
        "liquidityNet": "1198884537287",
        "tickIdx": "194810"
      },
      {
        "liquidityGross": "5471426786188891",
        "liquidityNet": "-5468120091862623",
        "tickIdx": "194820"
      },
      {
        "liquidityGross": "533647820144747",
        "liquidityNet": "195665307069921",
        "tickIdx": "194830"
      },
      {
        "liquidityGross": "668492964118805",
        "liquidityNet": "-665829839802697",
        "tickIdx": "194840"
      },
      {
        "liquidityGross": "5102194722791571",
        "liquidityNet": "-4942802848299371",
        "tickIdx": "194850"
      },
      {
        "liquidityGross": "32567901191946908",
        "liquidityNet": "-3697734260593614",
        "tickIdx": "194860"
      },
      {
        "liquidityGross": "287898898889255",
        "liquidityNet": "287898898889255",
        "tickIdx": "194880"
      },
      {
        "liquidityGross": "777118296410956",
        "liquidityNet": "777118296410956",
        "tickIdx": "194890"
      },
      {
        "liquidityGross": "749676584791462",
        "liquidityNet": "228341313864990",
        "tickIdx": "194900"
      },
      {
        "liquidityGross": "1102108361237794",
        "liquidityNet": "-1029688301426864",
        "tickIdx": "194910"
      },
      {
        "liquidityGross": "21992200782322085",
        "liquidityNet": "-21928990817041895",
        "tickIdx": "194920"
      },
      {
        "liquidityGross": "379597816286835",
        "liquidityNet": "-309460113585833",
        "tickIdx": "194930"
      },
      {
        "liquidityGross": "3231998783247424",
        "liquidityNet": "2077076889976162",
        "tickIdx": "194940"
      },
      {
        "liquidityGross": "1706669631022847",
        "liquidityNet": "669113382061557",
        "tickIdx": "194950"
      },
      {
        "liquidityGross": "1328439979490704",
        "liquidityNet": "-1328439979490704",
        "tickIdx": "194960"
      },
      {
        "liquidityGross": "6163535262852007",
        "liquidityNet": "5860387846202263",
        "tickIdx": "194970"
      },
      {
        "liquidityGross": "7751822118392542",
        "liquidityNet": "7751822118392542",
        "tickIdx": "194980"
      },
      {
        "liquidityGross": "6189175686659133",
        "liquidityNet": "-6137869534903127",
        "tickIdx": "194990"
      },
      {
        "liquidityGross": "62383734670543453",
        "liquidityNet": "-50389023348302991",
        "tickIdx": "195000"
      },
      {
        "liquidityGross": "130382076002772",
        "liquidityNet": "79076664559610",
        "tickIdx": "195010"
      },
      {
        "liquidityGross": "65811208806635",
        "liquidityNet": "55706007896799",
        "tickIdx": "195020"
      },
      {
        "liquidityGross": "801050594414955",
        "liquidityNet": "63890163133653",
        "tickIdx": "195030"
      },
      {
        "liquidityGross": "353365647023931",
        "liquidityNet": "-304458670649623",
        "tickIdx": "195040"
      },
      {
        "liquidityGross": "554871592058969",
        "liquidityNet": "554871592058969",
        "tickIdx": "195060"
      },
      {
        "liquidityGross": "239561679378639",
        "liquidityNet": "-189656584407909",
        "tickIdx": "195070"
      },
      {
        "liquidityGross": "2642135399259336",
        "liquidityNet": "1533469494789578",
        "tickIdx": "195080"
      },
      {
        "liquidityGross": "157651679368360",
        "liquidityNet": "107746584397630",
        "tickIdx": "195090"
      },
      {
        "liquidityGross": "497608501220997",
        "liquidityNet": "339824763520819",
        "tickIdx": "195100"
      },
      {
        "liquidityGross": "218847746587640",
        "liquidityNet": "-218847746587640",
        "tickIdx": "195110"
      },
      {
        "liquidityGross": "521465510876795",
        "liquidityNet": "-112839649712551",
        "tickIdx": "195120"
      },
      {
        "liquidityGross": "3562032315123960",
        "liquidityNet": "-3486121299590568",
        "tickIdx": "195130"
      },
      {
        "liquidityGross": "8620067529743194",
        "liquidityNet": "-8300737189398700",
        "tickIdx": "195140"
      },
      {
        "liquidityGross": "1969726067396043",
        "liquidityNet": "-1895451115356987",
        "tickIdx": "195150"
      },
      {
        "liquidityGross": "166415588589761",
        "liquidityNet": "166415588589761",
        "tickIdx": "195170"
      },
      {
        "liquidityGross": "301354185193029",
        "liquidityNet": "-294790772633901",
        "tickIdx": "195180"
      },
      {
        "liquidityGross": "835618767945628",
        "liquidityNet": "551258275208008",
        "tickIdx": "195190"
      },
      {
        "liquidityGross": "281983275787761",
        "liquidityNet": "164477651356407",
        "tickIdx": "195200"
      },
      {
        "liquidityGross": "2785453550",
        "liquidityNet": "-219252698",
        "tickIdx": "195210"
      },
      {
        "liquidityGross": "16791340254466096",
        "liquidityNet": "-16791310816433502",
        "tickIdx": "195220"
      },
      {
        "liquidityGross": "11739803578594",
        "liquidityNet": "11739803578594",
        "tickIdx": "195230"
      },
      {
        "liquidityGross": "15037593904798",
        "liquidityNet": "9244535614860",
        "tickIdx": "195240"
      },
      {
        "liquidityGross": "726147743046008",
        "liquidityNet": "-25784032552266",
        "tickIdx": "195250"
      },
      {
        "liquidityGross": "15982241300548",
        "liquidityNet": "15982241300548",
        "tickIdx": "195260"
      },
      {
        "liquidityGross": "345436228075577",
        "liquidityNet": "-345436228075577",
        "tickIdx": "195270"
      },
      {
        "liquidityGross": "138804658880979",
        "liquidityNet": "138804658880979",
        "tickIdx": "195280"
      },
      {
        "liquidityGross": "3280123399829202",
        "liquidityNet": "-1937970290003954",
        "tickIdx": "195300"
      },
      {
        "liquidityGross": "21524965643282282",
        "liquidityNet": "-21519444070932032",
        "tickIdx": "195310"
      },
      {
        "liquidityGross": "86097340223425",
        "liquidityNet": "-71769617341295",
        "tickIdx": "195320"
      },
      {
        "liquidityGross": "50869624045273",
        "liquidityNet": "50869624045273",
        "tickIdx": "195330"
      },
      {
        "liquidityGross": "8259866769861514",
        "liquidityNet": "-7493107812492592",
        "tickIdx": "195340"
      },
      {
        "liquidityGross": "845852564343001",
        "liquidityNet": "-731281869136289",
        "tickIdx": "195350"
      },
      {
        "liquidityGross": "95886477319952",
        "liquidityNet": "-56919965067774",
        "tickIdx": "195360"
      },
      {
        "liquidityGross": "3737381698644229",
        "liquidityNet": "-3412356995725307",
        "tickIdx": "195370"
      },
      {
        "liquidityGross": "104",
        "liquidityNet": "104",
        "tickIdx": "195380"
      },
      {
        "liquidityGross": "1497090499851427",
        "liquidityNet": "-1497090499851427",
        "tickIdx": "195390"
      },
      {
        "liquidityGross": "43243771706278",
        "liquidityNet": "43028994596340",
        "tickIdx": "195400"
      },
      {
        "liquidityGross": "1469886243943053",
        "liquidityNet": "-1469886243943053",
        "tickIdx": "195410"
      },
      {
        "liquidityGross": "34686419853248",
        "liquidityNet": "34686419853248",
        "tickIdx": "195420"
      },
      {
        "liquidityGross": "28404473623411",
        "liquidityNet": "-28404473623409",
        "tickIdx": "195430"
      },
      {
        "liquidityGross": "18177644570729552",
        "liquidityNet": "-18013437905850010",
        "tickIdx": "195440"
      },
      {
        "liquidityGross": "98203969114105",
        "liquidityNet": "98203969114105",
        "tickIdx": "195450"
      },
      {
        "liquidityGross": "1956783970846076",
        "liquidityNet": "1045070554901222",
        "tickIdx": "195460"
      },
      {
        "liquidityGross": "4045975464481646",
        "liquidityNet": "-4040590517588726",
        "tickIdx": "195470"
      },
      {
        "liquidityGross": "5551264933274766",
        "liquidityNet": "5020870184962128",
        "tickIdx": "195480"
      },
      {
        "liquidityGross": "185074376099300622",
        "liquidityNet": "-184637437819056102",
        "tickIdx": "195490"
      },
      {
        "liquidityGross": "95851646643317",
        "liquidityNet": "-95851646643317",
        "tickIdx": "195510"
      },
      {
        "liquidityGross": "5526124578130548",
        "liquidityNet": "-5264960535011316",
        "tickIdx": "195520"
      },
      {
        "liquidityGross": "24235385745561",
        "liquidityNet": "-24235298696341",
        "tickIdx": "195530"
      },
      {
        "liquidityGross": "550442191879715",
        "liquidityNet": "-550442191879715",
        "tickIdx": "195540"
      },
      {
        "liquidityGross": "2339352726316019",
        "liquidityNet": "650765631333407",
        "tickIdx": "195550"
      },
      {
        "liquidityGross": "21774841572416",
        "liquidityNet": "-17191670679762",
        "tickIdx": "195560"
      },
      {
        "liquidityGross": "16315284266143027",
        "liquidityNet": "13293650409287509",
        "tickIdx": "195570"
      },
      {
        "liquidityGross": "9672322328918",
        "liquidityNet": "9672322328918",
        "tickIdx": "195580"
      },
      {
        "liquidityGross": "16932070870753993",
        "liquidityNet": "-16154833820723031",
        "tickIdx": "195590"
      },
      {
        "liquidityGross": "3471003246113374",
        "liquidityNet": "-3379199545946558",
        "tickIdx": "195600"
      },
      {
        "liquidityGross": "140698396495948661",
        "liquidityNet": "-132739323543379259",
        "tickIdx": "195610"
      },
      {
        "liquidityGross": "42679098236154",
        "liquidityNet": "42679098236154",
        "tickIdx": "195620"
      },
      {
        "liquidityGross": "340629175831289",
        "liquidityNet": "340629175831289",
        "tickIdx": "195630"
      },
      {
        "liquidityGross": "128407175107591",
        "liquidityNet": "90919265604651",
        "tickIdx": "195640"
      },
      {
        "liquidityGross": "693045906459317",
        "liquidityNet": "-693045906459317",
        "tickIdx": "195650"
      },
      {
        "liquidityGross": "577880200972866",
        "liquidityNet": "567060156698400",
        "tickIdx": "195660"
      },
      {
        "liquidityGross": "3123686879969953",
        "liquidityNet": "-2715982272976903",
        "tickIdx": "195670"
      },
      {
        "liquidityGross": "4116513586312119",
        "liquidityNet": "-1192562086911467",
        "tickIdx": "195680"
      },
      {
        "liquidityGross": "726286635955495",
        "liquidityNet": "-724660478811199",
        "tickIdx": "195690"
      },
      {
        "liquidityGross": "202830781947565",
        "liquidityNet": "-201739572244771",
        "tickIdx": "195700"
      },
      {
        "liquidityGross": "44246660914013",
        "liquidityNet": "-42837753572237",
        "tickIdx": "195710"
      },
      {
        "liquidityGross": "4838970710323049",
        "liquidityNet": "3773059564357871",
        "tickIdx": "195720"
      },
      {
        "liquidityGross": "11267318795029853",
        "liquidityNet": "11232163953430129",
        "tickIdx": "195730"
      },
      {
        "liquidityGross": "1691898914474789",
        "liquidityNet": "1081912999085273",
        "tickIdx": "195740"
      },
      {
        "liquidityGross": "9043955485486811",
        "liquidityNet": "248973146536755",
        "tickIdx": "195750"
      },
      {
        "liquidityGross": "2433374422260672",
        "liquidityNet": "-1463744358332242",
        "tickIdx": "195760"
      },
      {
        "liquidityGross": "377261308026711",
        "liquidityNet": "100556077660499",
        "tickIdx": "195770"
      },
      {
        "liquidityGross": "1688492946991114",
        "liquidityNet": "1688492946991114",
        "tickIdx": "195780"
      },
      {
        "liquidityGross": "99956798298015",
        "liquidityNet": "5269229259151",
        "tickIdx": "195790"
      },
      {
        "liquidityGross": "144830091984295",
        "liquidityNet": "-127211881132285",
        "tickIdx": "195800"
      },
      {
        "liquidityGross": "5103041597563",
        "liquidityNet": "-5103041597563",
        "tickIdx": "195810"
      },
      {
        "liquidityGross": "866522758531286",
        "liquidityNet": "827459009295636",
        "tickIdx": "195820"
      },
      {
        "liquidityGross": "23741056796135",
        "liquidityNet": "-22701751625559",
        "tickIdx": "195830"
      },
      {
        "liquidityGross": "850447098745378",
        "liquidityNet": "-850447098745378",
        "tickIdx": "195840"
      },
      {
        "liquidityGross": "666237705107126",
        "liquidityNet": "666237705107126",
        "tickIdx": "195850"
      },
      {
        "liquidityGross": "14282369320847781",
        "liquidityNet": "-14263396463827633",
        "tickIdx": "195860"
      },
      {
        "liquidityGross": "559822598969316",
        "liquidityNet": "-559822598969316",
        "tickIdx": "195870"
      },
      {
        "liquidityGross": "2596369185381570",
        "liquidityNet": "-2067194179044762",
        "tickIdx": "195880"
      },
      {
        "liquidityGross": "14067997442399905",
        "liquidityNet": "14067997442399905",
        "tickIdx": "195890"
      },
      {
        "liquidityGross": "246502283733188",
        "liquidityNet": "-246502283733188",
        "tickIdx": "195900"
      },
      {
        "liquidityGross": "14125441610029806",
        "liquidityNet": "-14051275875585176",
        "tickIdx": "195910"
      },
      {
        "liquidityGross": "3212332789961123",
        "liquidityNet": "-2860221146779857",
        "tickIdx": "195920"
      },
      {
        "liquidityGross": "101564833960451310",
        "liquidityNet": "-93610427734633410",
        "tickIdx": "195930"
      },
      {
        "liquidityGross": "192540858111049",
        "liquidityNet": "-161643181407093",
        "tickIdx": "195940"
      },
      {
        "liquidityGross": "6922023355328583",
        "liquidityNet": "-2709659288287603",
        "tickIdx": "195960"
      },
      {
        "liquidityGross": "2358021752359031",
        "liquidityNet": "-2358021752359031",
        "tickIdx": "195970"
      },
      {
        "liquidityGross": "53411062339964950",
        "liquidityNet": "53411062339964950",
        "tickIdx": "195980"
      },
      {
        "liquidityGross": "2112913161804976",
        "liquidityNet": "-2112913161804976",
        "tickIdx": "195990"
      },
      {
        "liquidityGross": "532556565680575",
        "liquidityNet": "393152588963657",
        "tickIdx": "196000"
      },
      {
        "liquidityGross": "475866251593538",
        "liquidityNet": "-420244135655232",
        "tickIdx": "196010"
      },
      {
        "liquidityGross": "15038253251540",
        "liquidityNet": "-15038253251540",
        "tickIdx": "196020"
      },
      {
        "liquidityGross": "11924599853319737",
        "liquidityNet": "11849122343021879",
        "tickIdx": "196030"
      },
      {
        "liquidityGross": "3104434641388878",
        "liquidityNet": "2920714493200402",
        "tickIdx": "196040"
      },
      {
        "liquidityGross": "95276543486891",
        "liquidityNet": "-47835751939683",
        "tickIdx": "196050"
      },
      {
        "liquidityGross": "514824455657949",
        "liquidityNet": "-514782249692785",
        "tickIdx": "196060"
      },
      {
        "liquidityGross": "3035697820647806",
        "liquidityNet": "-2989451313941474",
        "tickIdx": "196070"
      },
      {
        "liquidityGross": "235123048042298",
        "liquidityNet": "235123048042298",
        "tickIdx": "196080"
      },
      {
        "liquidityGross": "11714823457488200",
        "liquidityNet": "-11659394408635524",
        "tickIdx": "196090"
      },
      {
        "liquidityGross": "233864248362155",
        "liquidityNet": "-233864248362155",
        "tickIdx": "196100"
      },
      {
        "liquidityGross": "1565522914726323",
        "liquidityNet": "-1565522914726323",
        "tickIdx": "196110"
      },
      {
        "liquidityGross": "326862961907812",
        "liquidityNet": "-284938284188386",
        "tickIdx": "196120"
      },
      {
        "liquidityGross": "549844851488473",
        "liquidityNet": "549844851488473",
        "tickIdx": "196130"
      },
      {
        "liquidityGross": "283035103384236",
        "liquidityNet": "281314102169220",
        "tickIdx": "196140"
      },
      {
        "liquidityGross": "566032523879",
        "liquidityNet": "566032523879",
        "tickIdx": "196150"
      },
      {
        "liquidityGross": "8940588416029748",
        "liquidityNet": "8567910539159240",
        "tickIdx": "196160"
      },
      {
        "liquidityGross": "5984457381009316245",
        "liquidityNet": "5984456248944268485",
        "tickIdx": "196170"
      },
      {
        "liquidityGross": "4373379595919090",
        "liquidityNet": "-4373379595919090",
        "tickIdx": "196180"
      },
      {
        "liquidityGross": "1561230648704025",
        "liquidityNet": "1443176916637037",
        "tickIdx": "196190"
      },
      {
        "liquidityGross": "39387742054624",
        "liquidityNet": "-4888017823148",
        "tickIdx": "196210"
      },
      {
        "liquidityGross": "4387101703966528",
        "liquidityNet": "-3986361749705394",
        "tickIdx": "196220"
      },
      {
        "liquidityGross": "89680474069800",
        "liquidityNet": "-89680474069800",
        "tickIdx": "196230"
      },
      {
        "liquidityGross": "271986028734444",
        "liquidityNet": "-270320681154154",
        "tickIdx": "196240"
      },
      {
        "liquidityGross": "6022981081053365645",
        "liquidityNet": "-6015260464613004827",
        "tickIdx": "196250"
      },
      {
        "liquidityGross": "36804521623326101",
        "liquidityNet": "-27179125210385857",
        "tickIdx": "196260"
      },
      {
        "liquidityGross": "257949016690013",
        "liquidityNet": "257949016690013",
        "tickIdx": "196270"
      },
      {
        "liquidityGross": "166878451509232",
        "liquidityNet": "101905185407398",
        "tickIdx": "196280"
      },
      {
        "liquidityGross": "8171423444647946",
        "liquidityNet": "-8171423444647946",
        "tickIdx": "196290"
      },
      {
        "liquidityGross": "2673159211186683",
        "liquidityNet": "689135043172139",
        "tickIdx": "196300"
      },
      {
        "liquidityGross": "87382429264762895",
        "liquidityNet": "-87368272458705571",
        "tickIdx": "196310"
      },
      {
        "liquidityGross": "97076682180177",
        "liquidityNet": "41675997802499",
        "tickIdx": "196320"
      },
      {
        "liquidityGross": "22733481403940",
        "liquidityNet": "-16206919685890",
        "tickIdx": "196330"
      },
      {
        "liquidityGross": "4367656344637662",
        "liquidityNet": "4148822298350800",
        "tickIdx": "196340"
      },
      {
        "liquidityGross": "7410015718579",
        "liquidityNet": "-7410015718579",
        "tickIdx": "196350"
      },
      {
        "liquidityGross": "44279667104524",
        "liquidityNet": "-44279667104524",
        "tickIdx": "196360"
      },
      {
        "liquidityGross": "68967570782517",
        "liquidityNet": "68967570782517",
        "tickIdx": "196380"
      },
      {
        "liquidityGross": "291722824867927",
        "liquidityNet": "291722824867927",
        "tickIdx": "196390"
      },
      {
        "liquidityGross": "260086033023964",
        "liquidityNet": "260077680036104",
        "tickIdx": "196400"
      },
      {
        "liquidityGross": "1462609585977549",
        "liquidityNet": "-1462609585977549",
        "tickIdx": "196410"
      },
      {
        "liquidityGross": "809017419016635",
        "liquidityNet": "-475536618132393",
        "tickIdx": "196420"
      },
      {
        "liquidityGross": "319748840686051",
        "liquidityNet": "319748840686051",
        "tickIdx": "196430"
      },
      {
        "liquidityGross": "174491192869730",
        "liquidityNet": "-174491192869730",
        "tickIdx": "196440"
      },
      {
        "liquidityGross": "482660906287241",
        "liquidityNet": "482660906287241",
        "tickIdx": "196450"
      },
      {
        "liquidityGross": "343465648843675",
        "liquidityNet": "-296032032528427",
        "tickIdx": "196460"
      },
      {
        "liquidityGross": "2098100836320611",
        "liquidityNet": "-2064473043840903",
        "tickIdx": "196470"
      },
      {
        "liquidityGross": "4796362650817581",
        "liquidityNet": "-4796362650817581",
        "tickIdx": "196480"
      },
      {
        "liquidityGross": "6960089675509779",
        "liquidityNet": "-6488441624916273",
        "tickIdx": "196490"
      },
      {
        "liquidityGross": "184913570239120",
        "liquidityNet": "184913570239120",
        "tickIdx": "196500"
      },
      {
        "liquidityGross": "744191556419164",
        "liquidityNet": "319528687331476",
        "tickIdx": "196510"
      },
      {
        "liquidityGross": "184913570239120",
        "liquidityNet": "-184913570239120",
        "tickIdx": "196520"
      },
      {
        "liquidityGross": "524376830331701",
        "liquidityNet": "-524376830331701",
        "tickIdx": "196530"
      },
      {
        "liquidityGross": "24897447029896",
        "liquidityNet": "-4699291558494",
        "tickIdx": "196540"
      },
      {
        "liquidityGross": "66336695746741",
        "liquidityNet": "-66336695746741",
        "tickIdx": "196550"
      },
      {
        "liquidityGross": "707259814897400",
        "liquidityNet": "687061659425998",
        "tickIdx": "196560"
      },
      {
        "liquidityGross": "543089335630363",
        "liquidityNet": "7334322586379",
        "tickIdx": "196570"
      },
      {
        "liquidityGross": "696421060987894",
        "liquidityNet": "-696421060987894",
        "tickIdx": "196580"
      },
      {
        "liquidityGross": "329215603081793",
        "liquidityNet": "-329215603081793",
        "tickIdx": "196590"
      },
      {
        "liquidityGross": "11238023438759517",
        "liquidityNet": "-6166231016028537",
        "tickIdx": "196600"
      },
      {
        "liquidityGross": "102569392222601",
        "liquidityNet": "-102569392222601",
        "tickIdx": "196610"
      },
      {
        "liquidityGross": "12671787221327446",
        "liquidityNet": "-12671787221327446",
        "tickIdx": "196620"
      },
      {
        "liquidityGross": "9287209971783",
        "liquidityNet": "7661052827487",
        "tickIdx": "196630"
      },
      {
        "liquidityGross": "3571023791649504",
        "liquidityNet": "2074098602687062",
        "tickIdx": "196640"
      },
      {
        "liquidityGross": "87029620885755",
        "liquidityNet": "-87029620885755",
        "tickIdx": "196650"
      },
      {
        "liquidityGross": "277416577183292",
        "liquidityNet": "-277416577183292",
        "tickIdx": "196660"
      },
      {
        "liquidityGross": "232856053400657",
        "liquidityNet": "-224623128936813",
        "tickIdx": "196680"
      },
      {
        "liquidityGross": "1990233101382540",
        "liquidityNet": "-1990233101382540",
        "tickIdx": "196690"
      },
      {
        "liquidityGross": "477264930847132",
        "liquidityNet": "477264930847132",
        "tickIdx": "196710"
      },
      {
        "liquidityGross": "459139595161597",
        "liquidityNet": "243476404405577",
        "tickIdx": "196720"
      },
      {
        "liquidityGross": "68112018101153",
        "liquidityNet": "68095834915471",
        "tickIdx": "196730"
      },
      {
        "liquidityGross": "4931510757581690",
        "liquidityNet": "-4931510757581690",
        "tickIdx": "196740"
      },
      {
        "liquidityGross": "6041575136322",
        "liquidityNet": "-6041575136322",
        "tickIdx": "196750"
      },
      {
        "liquidityGross": "272029515528899",
        "liquidityNet": "-269925483913323",
        "tickIdx": "196760"
      },
      {
        "liquidityGross": "978379191344196",
        "liquidityNet": "-528217314752672",
        "tickIdx": "196770"
      },
      {
        "liquidityGross": "888202507359966",
        "liquidityNet": "882766914253146",
        "tickIdx": "196780"
      },
      {
        "liquidityGross": "1068877492597577",
        "liquidityNet": "1063903322650331",
        "tickIdx": "196790"
      },
      {
        "liquidityGross": "1320579380532456",
        "liquidityNet": "-1320579380532456",
        "tickIdx": "196800"
      },
      {
        "liquidityGross": "21951217748923232",
        "liquidityNet": "20516980910172768",
        "tickIdx": "196810"
      },
      {
        "liquidityGross": "6745355184540147",
        "liquidityNet": "6500333245063351",
        "tickIdx": "196820"
      },
      {
        "liquidityGross": "2876512332",
        "liquidityNet": "-14334110",
        "tickIdx": "196840"
      },
      {
        "liquidityGross": "56882",
        "liquidityNet": "56882",
        "tickIdx": "196850"
      },
      {
        "liquidityGross": "383169952298211",
        "liquidityNet": "383169952298211",
        "tickIdx": "196860"
      },
      {
        "liquidityGross": "10135906124416",
        "liquidityNet": "10135906124416",
        "tickIdx": "196870"
      },
      {
        "liquidityGross": "84985685677042",
        "liquidityNet": "-62329623533738",
        "tickIdx": "196880"
      },
      {
        "liquidityGross": "2665836309138757",
        "liquidityNet": "2660309908620727",
        "tickIdx": "196900"
      },
      {
        "liquidityGross": "821924047863187",
        "liquidityNet": "-821190629432413",
        "tickIdx": "196910"
      },
      {
        "liquidityGross": "59208377072393",
        "liquidityNet": "58484208444917",
        "tickIdx": "196920"
      },
      {
        "liquidityGross": "1460551175454691",
        "liquidityNet": "-1460551175454691",
        "tickIdx": "196930"
      },
      {
        "liquidityGross": "601005627375049",
        "liquidityNet": "531758138094093",
        "tickIdx": "196940"
      },
      {
        "liquidityGross": "13194736172637462",
        "liquidityNet": "-12676721387145060",
        "tickIdx": "196950"
      },
      {
        "liquidityGross": "43873385375806",
        "liquidityNet": "-43873385375806",
        "tickIdx": "196960"
      },
      {
        "liquidityGross": "2892145283978544",
        "liquidityNet": "-789901818128012",
        "tickIdx": "196970"
      },
      {
        "liquidityGross": "3959478091923",
        "liquidityNet": "3959478091923",
        "tickIdx": "196980"
      },
      {
        "liquidityGross": "1021411710393621",
        "liquidityNet": "-1021236380313973",
        "tickIdx": "196990"
      },
      {
        "liquidityGross": "4787859316837343",
        "liquidityNet": "4259862112063249",
        "tickIdx": "197000"
      },
      {
        "liquidityGross": "6987350609481000",
        "liquidityNet": "-6986023507284608",
        "tickIdx": "197010"
      },
      {
        "liquidityGross": "11430936968622",
        "liquidityNet": "-11430841448312",
        "tickIdx": "197020"
      },
      {
        "liquidityGross": "3988174290757047",
        "liquidityNet": "1607497067360447",
        "tickIdx": "197030"
      },
      {
        "liquidityGross": "2050030463043466",
        "liquidityNet": "2050030367523156",
        "tickIdx": "197040"
      },
      {
        "liquidityGross": "99794828693738",
        "liquidityNet": "-85099879393624",
        "tickIdx": "197050"
      },
      {
        "liquidityGross": "30422510359689",
        "liquidityNet": "30422423310469",
        "tickIdx": "197060"
      },
      {
        "liquidityGross": "133917883322390",
        "liquidityNet": "-130633561795810",
        "tickIdx": "197070"
      },
      {
        "liquidityGross": "122140198278262",
        "liquidityNet": "-108838902558678",
        "tickIdx": "197080"
      },
      {
        "liquidityGross": "11845915314244",
        "liquidityNet": "8561593787664",
        "tickIdx": "197090"
      },
      {
        "liquidityGross": "5880440729039",
        "liquidityNet": "-5880440729039",
        "tickIdx": "197100"
      },
      {
        "liquidityGross": "44645831046936",
        "liquidityNet": "-44645831046936",
        "tickIdx": "197110"
      },
      {
        "liquidityGross": "36695194194845519",
        "liquidityNet": "-32698061631415449",
        "tickIdx": "197130"
      },
      {
        "liquidityGross": "54260249526638919",
        "liquidityNet": "-51451484179696597",
        "tickIdx": "197140"
      },
      {
        "liquidityGross": "130374838736715",
        "liquidityNet": "129298387518447",
        "tickIdx": "197150"
      },
      {
        "liquidityGross": "28537188295320700",
        "liquidityNet": "28056258430032026",
        "tickIdx": "197160"
      },
      {
        "liquidityGross": "1379369439272641",
        "liquidityNet": "1379369439272641",
        "tickIdx": "197180"
      },
      {
        "liquidityGross": "74240964321383",
        "liquidityNet": "-74240964321383",
        "tickIdx": "197190"
      },
      {
        "liquidityGross": "1994218449546505",
        "liquidityNet": "-1994213403985635",
        "tickIdx": "197200"
      },
      {
        "liquidityGross": "71768594532802",
        "liquidityNet": "32576198885432",
        "tickIdx": "197220"
      },
      {
        "liquidityGross": "107963108977593",
        "liquidityNet": "107746496109199",
        "tickIdx": "197230"
      },
      {
        "liquidityGross": "331099282921941",
        "liquidityNet": "212608305582293",
        "tickIdx": "197240"
      },
      {
        "liquidityGross": "552439640899999",
        "liquidityNet": "-499184970077157",
        "tickIdx": "197250"
      },
      {
        "liquidityGross": "440729495904263",
        "liquidityNet": "-102978092599971",
        "tickIdx": "197260"
      },
      {
        "liquidityGross": "344897400174848",
        "liquidityNet": "299117231476348",
        "tickIdx": "197270"
      },
      {
        "liquidityGross": "322824870534948",
        "liquidityNet": "-321189761116248",
        "tickIdx": "197290"
      },
      {
        "liquidityGross": "49847865154498154",
        "liquidityNet": "-43089162131875858",
        "tickIdx": "197310"
      },
      {
        "liquidityGross": "390066994717598",
        "liquidityNet": "390066994717598",
        "tickIdx": "197330"
      },
      {
        "liquidityGross": "276127971788189",
        "liquidityNet": "81755946865973",
        "tickIdx": "197340"
      },
      {
        "liquidityGross": "306545319068908",
        "liquidityNet": "-225380356684722",
        "tickIdx": "197350"
      },
      {
        "liquidityGross": "86730202507546",
        "liquidityNet": "27032107398706",
        "tickIdx": "197360"
      },
      {
        "liquidityGross": "181622011108645",
        "liquidityNet": "181622011108645",
        "tickIdx": "197370"
      },
      {
        "liquidityGross": "1767892989867346",
        "liquidityNet": "-1007482363660684",
        "tickIdx": "197380"
      },
      {
        "liquidityGross": "32312579310165",
        "liquidityNet": "-21010074419059",
        "tickIdx": "197390"
      },
      {
        "liquidityGross": "5248503423456263",
        "liquidityNet": "5248503423456263",
        "tickIdx": "197400"
      },
      {
        "liquidityGross": "569251602324328",
        "liquidityNet": "557949097433222",
        "tickIdx": "197410"
      },
      {
        "liquidityGross": "1640812164371685",
        "liquidityNet": "-427386792905201",
        "tickIdx": "197420"
      },
      {
        "liquidityGross": "2957802016056091",
        "liquidityNet": "2694300761945565",
        "tickIdx": "197430"
      },
      {
        "liquidityGross": "4761210274778154",
        "liquidityNet": "-4726664508002426",
        "tickIdx": "197440"
      },
      {
        "liquidityGross": "24006119117640",
        "liquidityNet": "-23868680164168",
        "tickIdx": "197450"
      },
      {
        "liquidityGross": "1406036504377436",
        "liquidityNet": "-1398454345514152",
        "tickIdx": "197460"
      },
      {
        "liquidityGross": "4528639197025325",
        "liquidityNet": "1634428937928623",
        "tickIdx": "197470"
      },
      {
        "liquidityGross": "3073114744199753",
        "liquidityNet": "-3073114744199753",
        "tickIdx": "197480"
      },
      {
        "liquidityGross": "12210402708863",
        "liquidityNet": "-12210402708863",
        "tickIdx": "197490"
      },
      {
        "liquidityGross": "20897196628272418",
        "liquidityNet": "-20681689218951872",
        "tickIdx": "197500"
      },
      {
        "liquidityGross": "266471630607",
        "liquidityNet": "-251054225367",
        "tickIdx": "197510"
      },
      {
        "liquidityGross": "361414005157098",
        "liquidityNet": "361346765013654",
        "tickIdx": "197520"
      },
      {
        "liquidityGross": "21435120844932362",
        "liquidityNet": "-21033077814163638",
        "tickIdx": "197530"
      },
      {
        "liquidityGross": "455024087853946",
        "liquidityNet": "-268815166292558",
        "tickIdx": "197540"
      },
      {
        "liquidityGross": "195608658405383",
        "liquidityNet": "-195608658405383",
        "tickIdx": "197550"
      },
      {
        "liquidityGross": "10976623986803697",
        "liquidityNet": "10976623986803697",
        "tickIdx": "197560"
      },
      {
        "liquidityGross": "1409182658129687",
        "liquidityNet": "1409179795951465",
        "tickIdx": "197570"
      },
      {
        "liquidityGross": "11239229093602154",
        "liquidityNet": "-11037147452304866",
        "tickIdx": "197580"
      },
      {
        "liquidityGross": "895723459457961",
        "liquidityNet": "-892169573277407",
        "tickIdx": "197590"
      },
      {
        "liquidityGross": "1954797849023913",
        "liquidityNet": "1631481705377575",
        "tickIdx": "197600"
      },
      {
        "liquidityGross": "167603524454094",
        "liquidityNet": "167583061487984",
        "tickIdx": "197610"
      },
      {
        "liquidityGross": "1866846556474383",
        "liquidityNet": "-1847429021911883",
        "tickIdx": "197620"
      },
      {
        "liquidityGross": "193739327053775",
        "liquidityNet": "193739327053775",
        "tickIdx": "197630"
      },
      {
        "liquidityGross": "207591694004054",
        "liquidityNet": "207591694004054",
        "tickIdx": "197640"
      },
      {
        "liquidityGross": "4799140240443835",
        "liquidityNet": "4126567458044491",
        "tickIdx": "197650"
      },
      {
        "liquidityGross": "197371060368821",
        "liquidityNet": "-197248894708313",
        "tickIdx": "197660"
      },
      {
        "liquidityGross": "4558810721498131",
        "liquidityNet": "-4376644206972401",
        "tickIdx": "197670"
      },
      {
        "liquidityGross": "2035441935076012",
        "liquidityNet": "81598786816922",
        "tickIdx": "197680"
      },
      {
        "liquidityGross": "38127508600914737",
        "liquidityNet": "-35158579729713989",
        "tickIdx": "197690"
      },
      {
        "liquidityGross": "2285650019145291",
        "liquidityNet": "66596947231437",
        "tickIdx": "197700"
      },
      {
        "liquidityGross": "40188748975509",
        "liquidityNet": "26307506116817",
        "tickIdx": "197710"
      },
      {
        "liquidityGross": "1396944453892922",
        "liquidityNet": "-912133972808446",
        "tickIdx": "197720"
      },
      {
        "liquidityGross": "2229990130976303198",
        "liquidityNet": "2229682829134415838",
        "tickIdx": "197730"
      },
      {
        "liquidityGross": "2232467168869842952",
        "liquidityNet": "-2227345450299561462",
        "tickIdx": "197740"
      },
      {
        "liquidityGross": "4305006590010615",
        "liquidityNet": "-2780159707162433",
        "tickIdx": "197750"
      },
      {
        "liquidityGross": "38002350481944",
        "liquidityNet": "13329787193048",
        "tickIdx": "197760"
      },
      {
        "liquidityGross": "1141479963597901",
        "liquidityNet": "-383366919250281",
        "tickIdx": "197780"
      },
      {
        "liquidityGross": "96611392466032",
        "liquidityNet": "56015721937652",
        "tickIdx": "197790"
      },
      {
        "liquidityGross": "1316508517379869",
        "liquidityNet": "1316508517379869",
        "tickIdx": "197800"
      },
      {
        "liquidityGross": "3807499523282080",
        "liquidityNet": "-3807499523282080",
        "tickIdx": "197810"
      },
      {
        "liquidityGross": "99047449833039",
        "liquidityNet": "-13588590519967",
        "tickIdx": "197820"
      },
      {
        "liquidityGross": "1338326363860077",
        "liquidityNet": "-1293418572576637",
        "tickIdx": "197830"
      },
      {
        "liquidityGross": "10463644269478",
        "liquidityNet": "-10463644269478",
        "tickIdx": "197840"
      },
      {
        "liquidityGross": "12141064759829",
        "liquidityNet": "-12141064759829",
        "tickIdx": "197850"
      },
      {
        "liquidityGross": "6601407800121139",
        "liquidityNet": "-6581266493858149",
        "tickIdx": "197860"
      },
      {
        "liquidityGross": "879629239876411",
        "liquidityNet": "395586090970563",
        "tickIdx": "197870"
      },
      {
        "liquidityGross": "32033490441974236",
        "liquidityNet": "-31919229822004590",
        "tickIdx": "197880"
      },
      {
        "liquidityGross": "1482776574796004",
        "liquidityNet": "-1482739357178268",
        "tickIdx": "197890"
      },
      {
        "liquidityGross": "320436328606285",
        "liquidityNet": "139641090876619",
        "tickIdx": "197900"
      },
      {
        "liquidityGross": "3598490209837783",
        "liquidityNet": "3598490209837783",
        "tickIdx": "197910"
      },
      {
        "liquidityGross": "241455990057546",
        "liquidityNet": "-152656005780136",
        "tickIdx": "197920"
      },
      {
        "liquidityGross": "34294593581126",
        "liquidityNet": "-13554370420632",
        "tickIdx": "197930"
      },
      {
        "liquidityGross": "3377080657034663",
        "liquidityNet": "-3373078624420383",
        "tickIdx": "197940"
      },
      {
        "liquidityGross": "1595246119253323",
        "liquidityNet": "1113055660529935",
        "tickIdx": "197950"
      },
      {
        "liquidityGross": "2001016307140",
        "liquidityNet": "-2001016307140",
        "tickIdx": "197960"
      },
      {
        "liquidityGross": "2990371534892578",
        "liquidityNet": "-2153511778279878",
        "tickIdx": "197970"
      },
      {
        "liquidityGross": "1034981708568469",
        "liquidityNet": "-913943380727185",
        "tickIdx": "197980"
      },
      {
        "liquidityGross": "286205815959",
        "liquidityNet": "272092801187",
        "tickIdx": "197990"
      },
      {
        "liquidityGross": "261896816185174",
        "liquidityNet": "208621214404126",
        "tickIdx": "198000"
      },
      {
        "liquidityGross": "963001173437481",
        "liquidityNet": "-936079229751201",
        "tickIdx": "198010"
      },
      {
        "liquidityGross": "17055617782345",
        "liquidityNet": "7252688040227",
        "tickIdx": "198020"
      },
      {
        "liquidityGross": "395005360439446",
        "liquidityNet": "338079598778340",
        "tickIdx": "198030"
      },
      {
        "liquidityGross": "1173397557647392",
        "liquidityNet": "554072537741302",
        "tickIdx": "198040"
      },
      {
        "liquidityGross": "1453610403335933",
        "liquidityNet": "-1453610403335933",
        "tickIdx": "198050"
      },
      {
        "liquidityGross": "836882683461998",
        "liquidityNet": "-836882683461998",
        "tickIdx": "198060"
      },
      {
        "liquidityGross": "44309445206565",
        "liquidityNet": "-44309445206565",
        "tickIdx": "198070"
      },
      {
        "liquidityGross": "61088647182125764",
        "liquidityNet": "-55625541478295428",
        "tickIdx": "198080"
      },
      {
        "liquidityGross": "39383549767089602",
        "liquidityNet": "-39383549767089602",
        "tickIdx": "198090"
      },
      {
        "liquidityGross": "39163878708344765",
        "liquidityNet": "8743237610319699",
        "tickIdx": "198100"
      },
      {
        "liquidityGross": "15730064624215538",
        "liquidityNet": "-15497856512577588",
        "tickIdx": "198120"
      },
      {
        "liquidityGross": "24069662215151207",
        "liquidityNet": "-24069662215151207",
        "tickIdx": "198130"
      },
      {
        "liquidityGross": "2821666989886645",
        "liquidityNet": "-2818785958309855",
        "tickIdx": "198140"
      },
      {
        "liquidityGross": "551816231263643",
        "liquidityNet": "479779145875449",
        "tickIdx": "198160"
      },
      {
        "liquidityGross": "16507237651455",
        "liquidityNet": "-16507237651455",
        "tickIdx": "198170"
      },
      {
        "liquidityGross": "1014981683082535",
        "liquidityNet": "-20815245879999",
        "tickIdx": "198180"
      },
      {
        "liquidityGross": "579093774183822",
        "liquidityNet": "517606464452634",
        "tickIdx": "198190"
      },
      {
        "liquidityGross": "2492961192862707",
        "liquidityNet": "628650267437247",
        "tickIdx": "198200"
      },
      {
        "liquidityGross": "1611989188706113",
        "liquidityNet": "-1564618024823671",
        "tickIdx": "198210"
      },
      {
        "liquidityGross": "1009880360948969",
        "liquidityNet": "-403734916652805",
        "tickIdx": "198230"
      },
      {
        "liquidityGross": "2519036035195",
        "liquidityNet": "2519036035195",
        "tickIdx": "198240"
      },
      {
        "liquidityGross": "562444160992080",
        "liquidityNet": "-562444160992080",
        "tickIdx": "198250"
      },
      {
        "liquidityGross": "106963752959853",
        "liquidityNet": "-54422833079581",
        "tickIdx": "198260"
      },
      {
        "liquidityGross": "71950050520113",
        "liquidityNet": "-71950050520113",
        "tickIdx": "198270"
      },
      {
        "liquidityGross": "123417252049990",
        "liquidityNet": "-73392587069442",
        "tickIdx": "198280"
      },
      {
        "liquidityGross": "47739347251",
        "liquidityNet": "47739347251",
        "tickIdx": "198290"
      },
      {
        "liquidityGross": "40947929943861",
        "liquidityNet": "-31838653988823",
        "tickIdx": "198300"
      },
      {
        "liquidityGross": "18157443316294683",
        "liquidityNet": "18157443316294683",
        "tickIdx": "198310"
      },
      {
        "liquidityGross": "1163714801039019",
        "liquidityNet": "1131825612344787",
        "tickIdx": "198320"
      },
      {
        "liquidityGross": "17908956119986410",
        "liquidityNet": "-17908294449575416",
        "tickIdx": "198330"
      },
      {
        "liquidityGross": "1147770206691903",
        "liquidityNet": "-1147770206691903",
        "tickIdx": "198340"
      },
      {
        "liquidityGross": "245592076582059",
        "liquidityNet": "245592076582059",
        "tickIdx": "198360"
      },
      {
        "liquidityGross": "826601339",
        "liquidityNet": "-826601339",
        "tickIdx": "198380"
      },
      {
        "liquidityGross": "719385737917328",
        "liquidityNet": "719385737917328",
        "tickIdx": "198390"
      },
      {
        "liquidityGross": "15580378389654",
        "liquidityNet": "15580378389654",
        "tickIdx": "198400"
      },
      {
        "liquidityGross": "2124157394070",
        "liquidityNet": "-2124157394070",
        "tickIdx": "198410"
      },
      {
        "liquidityGross": "468542124700012",
        "liquidityNet": "451500832683482",
        "tickIdx": "198420"
      },
      {
        "liquidityGross": "323621183209916",
        "liquidityNet": "283463112072528",
        "tickIdx": "198430"
      },
      {
        "liquidityGross": "909590861292435",
        "liquidityNet": "583919962818375",
        "tickIdx": "198440"
      },
      {
        "liquidityGross": "290277545196536",
        "liquidityNet": "-290277545196536",
        "tickIdx": "198450"
      },
      {
        "liquidityGross": "3317110763323210",
        "liquidityNet": "1776166322897152",
        "tickIdx": "198460"
      },
      {
        "liquidityGross": "99862653496",
        "liquidityNet": "99862653496",
        "tickIdx": "198470"
      },
      {
        "liquidityGross": "906786425592605",
        "liquidityNet": "899580297516121",
        "tickIdx": "198480"
      },
      {
        "liquidityGross": "11132402580271691",
        "liquidityNet": "-9438298501242043",
        "tickIdx": "198490"
      },
      {
        "liquidityGross": "7357329207108820",
        "liquidityNet": "-7357329207108820",
        "tickIdx": "198500"
      },
      {
        "liquidityGross": "212678241574632",
        "liquidityNet": "-11496411404286",
        "tickIdx": "198510"
      },
      {
        "liquidityGross": "497339885311537",
        "liquidityNet": "-497339885311537",
        "tickIdx": "198530"
      },
      {
        "liquidityGross": "173732600262594",
        "liquidityNet": "173732600262594",
        "tickIdx": "198540"
      },
      {
        "liquidityGross": "165264207447947",
        "liquidityNet": "-165264207447947",
        "tickIdx": "198560"
      },
      {
        "liquidityGross": "3791691654104524",
        "liquidityNet": "3776140879709132",
        "tickIdx": "198570"
      },
      {
        "liquidityGross": "5549435394448",
        "liquidityNet": "-5549435394448",
        "tickIdx": "198580"
      },
      {
        "liquidityGross": "3968797884756267",
        "liquidityNet": "-3968797884756267",
        "tickIdx": "198590"
      },
      {
        "liquidityGross": "25666701827351",
        "liquidityNet": "-25666701827351",
        "tickIdx": "198610"
      },
      {
        "liquidityGross": "282817264728283",
        "liquidityNet": "282817264728283",
        "tickIdx": "198620"
      },
      {
        "liquidityGross": "237158434977170",
        "liquidityNet": "231257853830576",
        "tickIdx": "198630"
      },
      {
        "liquidityGross": "13283408140229",
        "liquidityNet": "-13283408140229",
        "tickIdx": "198640"
      },
      {
        "liquidityGross": "230133082661011",
        "liquidityNet": "-230133082661011",
        "tickIdx": "198650"
      },
      {
        "liquidityGross": "65006435935919",
        "liquidityNet": "56773511472075",
        "tickIdx": "198660"
      },
      {
        "liquidityGross": "93068856583417",
        "liquidityNet": "-93068856583417",
        "tickIdx": "198670"
      },
      {
        "liquidityGross": "10568367650",
        "liquidityNet": "-10568367650",
        "tickIdx": "198680"
      },
      {
        "liquidityGross": "273081710105998",
        "liquidityNet": "92651462201874",
        "tickIdx": "198690"
      },
      {
        "liquidityGross": "341801951139496",
        "liquidityNet": "-341798153045906",
        "tickIdx": "198700"
      },
      {
        "liquidityGross": "1707563351284302",
        "liquidityNet": "297152874873684",
        "tickIdx": "198710"
      },
      {
        "liquidityGross": "1329378141046676",
        "liquidityNet": "-1329378141046676",
        "tickIdx": "198720"
      },
      {
        "liquidityGross": "4291025125981313",
        "liquidityNet": "-4291025125981313",
        "tickIdx": "198730"
      },
      {
        "liquidityGross": "104200415654279",
        "liquidityNet": "-32014372783357",
        "tickIdx": "198740"
      },
      {
        "liquidityGross": "6294244116656452",
        "liquidityNet": "6293937469386956",
        "tickIdx": "198750"
      },
      {
        "liquidityGross": "6494577022514601",
        "liquidityNet": "-6474856383277037",
        "tickIdx": "198760"
      },
      {
        "liquidityGross": "46166591212031",
        "liquidityNet": "-45918026204723",
        "tickIdx": "198770"
      },
      {
        "liquidityGross": "198152867198909",
        "liquidityNet": "37834206153297",
        "tickIdx": "198780"
      },
      {
        "liquidityGross": "76273173257762",
        "liquidityNet": "-76273173257762",
        "tickIdx": "198790"
      },
      {
        "liquidityGross": "8372045892911",
        "liquidityNet": "-8372045892911",
        "tickIdx": "198810"
      },
      {
        "liquidityGross": "360001813611171",
        "liquidityNet": "-360001813611171",
        "tickIdx": "198820"
      },
      {
        "liquidityGross": "2797835679058747",
        "liquidityNet": "-2797835679058747",
        "tickIdx": "198830"
      },
      {
        "liquidityGross": "4029643996556",
        "liquidityNet": "-4029643996556",
        "tickIdx": "198840"
      },
      {
        "liquidityGross": "34485672668104",
        "liquidityNet": "-34485672668104",
        "tickIdx": "198850"
      },
      {
        "liquidityGross": "598528129420",
        "liquidityNet": "-598528128940",
        "tickIdx": "198860"
      },
      {
        "liquidityGross": "97104963908898",
        "liquidityNet": "-97104963908898",
        "tickIdx": "198870"
      },
      {
        "liquidityGross": "499528760799264",
        "liquidityNet": "318751421923936",
        "tickIdx": "198880"
      },
      {
        "liquidityGross": "464608994416258",
        "liquidityNet": "464608994416258",
        "tickIdx": "198890"
      },
      {
        "liquidityGross": "959622567568204",
        "liquidityNet": "959622567567724",
        "tickIdx": "198900"
      },
      {
        "liquidityGross": "8972329658921283",
        "liquidityNet": "-6987721747465759",
        "tickIdx": "198910"
      },
      {
        "liquidityGross": "4009505580216188",
        "liquidityNet": "-2644536673688760",
        "tickIdx": "198920"
      },
      {
        "liquidityGross": "9432688312744859",
        "liquidityNet": "9432688312744859",
        "tickIdx": "198930"
      },
      {
        "liquidityGross": "61455478067672",
        "liquidityNet": "-61455478067194",
        "tickIdx": "198940"
      },
      {
        "liquidityGross": "9523392769960157",
        "liquidityNet": "-9523392769960157",
        "tickIdx": "198950"
      },
      {
        "liquidityGross": "671785593750083",
        "liquidityNet": "-656407899567235",
        "tickIdx": "198960"
      },
      {
        "liquidityGross": "11600521422629",
        "liquidityNet": "-11600521422629",
        "tickIdx": "198970"
      },
      {
        "liquidityGross": "6057272388709",
        "liquidityNet": "-6057272388149",
        "tickIdx": "198980"
      },
      {
        "liquidityGross": "116724181483482",
        "liquidityNet": "116038509072488",
        "tickIdx": "198990"
      },
      {
        "liquidityGross": "2941147222023",
        "liquidityNet": "2941147221545",
        "tickIdx": "199000"
      },
      {
        "liquidityGross": "116381345277985",
        "liquidityNet": "-116381345277985",
        "tickIdx": "199010"
      },
      {
        "liquidityGross": "97528009934540",
        "liquidityNet": "96200907737588",
        "tickIdx": "199020"
      },
      {
        "liquidityGross": "99170907039791",
        "liquidityNet": "62809852281145",
        "tickIdx": "199030"
      },
      {
        "liquidityGross": "1608314815216",
        "liquidityNet": "-1608314815216",
        "tickIdx": "199040"
      },
      {
        "liquidityGross": "559759276453364",
        "liquidityNet": "-559759276453364",
        "tickIdx": "199050"
      },
      {
        "liquidityGross": "82545145586010",
        "liquidityNet": "-82545145586010",
        "tickIdx": "199060"
      },
      {
        "liquidityGross": "73788088044",
        "liquidityNet": "73788088044",
        "tickIdx": "199070"
      },
      {
        "liquidityGross": "4386037171978715",
        "liquidityNet": "4386037171978715",
        "tickIdx": "199080"
      },
      {
        "liquidityGross": "155528062251908",
        "liquidityNet": "155380486075820",
        "tickIdx": "199090"
      },
      {
        "liquidityGross": "19048869057949",
        "liquidityNet": "-19048869057949",
        "tickIdx": "199100"
      },
      {
        "liquidityGross": "27615798513471246",
        "liquidityNet": "26941645942926228",
        "tickIdx": "199110"
      },
      {
        "liquidityGross": "2309155060695394",
        "liquidityNet": "2214819435643762",
        "tickIdx": "199130"
      },
      {
        "liquidityGross": "7074371622550205",
        "liquidityNet": "6430302596776479",
        "tickIdx": "199140"
      },
      {
        "liquidityGross": "47373281153672",
        "liquidityNet": "47373281153672",
        "tickIdx": "199150"
      },
      {
        "liquidityGross": "7034769316039784",
        "liquidityNet": "-6469904903001750",
        "tickIdx": "199160"
      },
      {
        "liquidityGross": "642248387401628",
        "liquidityNet": "-575524103097152",
        "tickIdx": "199170"
      },
      {
        "liquidityGross": "75621400390424175",
        "liquidityNet": "-75621400390424175",
        "tickIdx": "199180"
      },
      {
        "liquidityGross": "299319372937502",
        "liquidityNet": "232595088633026",
        "tickIdx": "199190"
      },
      {
        "liquidityGross": "39201145479371879",
        "liquidityNet": "30587313534450029",
        "tickIdx": "199200"
      },
      {
        "liquidityGross": "275753205373460",
        "liquidityNet": "-256161256197068",
        "tickIdx": "199210"
      },
      {
        "liquidityGross": "49285087840171",
        "liquidityNet": "49285086809113",
        "tickIdx": "199220"
      },
      {
        "liquidityGross": "154103332786117",
        "liquidityNet": "141245037263931",
        "tickIdx": "199230"
      },
      {
        "liquidityGross": "14164582632099",
        "liquidityNet": "14164582632099",
        "tickIdx": "199240"
      },
      {
        "liquidityGross": "1943675254506722",
        "liquidityNet": "1927115828601846",
        "tickIdx": "199250"
      },
      {
        "liquidityGross": "26917912910260041",
        "liquidityNet": "23965416244048219",
        "tickIdx": "199260"
      },
      {
        "liquidityGross": "1874510080240612",
        "liquidityNet": "-1874510080240612",
        "tickIdx": "199270"
      },
      {
        "liquidityGross": "1992932662768916",
        "liquidityNet": "-1992932662768916",
        "tickIdx": "199280"
      },
      {
        "liquidityGross": "148016922752040",
        "liquidityNet": "148016922752040",
        "tickIdx": "199290"
      },
      {
        "liquidityGross": "393562713808220",
        "liquidityNet": "-353647815938132",
        "tickIdx": "199300"
      },
      {
        "liquidityGross": "1352885195903299456",
        "liquidityNet": "1352885195903299456",
        "tickIdx": "199310"
      },
      {
        "liquidityGross": "34032520525706223",
        "liquidityNet": "-33519855362526023",
        "tickIdx": "199320"
      },
      {
        "liquidityGross": "70658562168873",
        "liquidityNet": "14346432094203",
        "tickIdx": "199330"
      },
      {
        "liquidityGross": "817554709350",
        "liquidityNet": "-817554709350",
        "tickIdx": "199340"
      },
      {
        "liquidityGross": "792317939497576",
        "liquidityNet": "-792317939497576",
        "tickIdx": "199350"
      },
      {
        "liquidityGross": "7714405298723543",
        "liquidityNet": "-7305128034384889",
        "tickIdx": "199360"
      },
      {
        "liquidityGross": "6285130699875",
        "liquidityNet": "-4642147956327",
        "tickIdx": "199370"
      },
      {
        "liquidityGross": "7598705807111945502",
        "liquidityNet": "7598705807111945502",
        "tickIdx": "199380"
      },
      {
        "liquidityGross": "6908484258181",
        "liquidityNet": "-6908484258181",
        "tickIdx": "199390"
      },
      {
        "liquidityGross": "233",
        "liquidityNet": "233",
        "tickIdx": "199400"
      },
      {
        "liquidityGross": "467",
        "liquidityNet": "-1",
        "tickIdx": "199420"
      },
      {
        "liquidityGross": "16427088716168126",
        "liquidityNet": "16427088716168126",
        "tickIdx": "199430"
      },
      {
        "liquidityGross": "1260911507144698",
        "liquidityNet": "1165566570179008",
        "tickIdx": "199440"
      },
      {
        "liquidityGross": "600001754019951",
        "liquidityNet": "-600001754019951",
        "tickIdx": "199450"
      },
      {
        "liquidityGross": "16404462357410162",
        "liquidityNet": "-16404462357409696",
        "tickIdx": "199460"
      },
      {
        "liquidityGross": "1364262192671213",
        "liquidityNet": "-1111237339896895",
        "tickIdx": "199470"
      },
      {
        "liquidityGross": "33324673217",
        "liquidityNet": "-33324672753",
        "tickIdx": "199480"
      },
      {
        "liquidityGross": "109687086610797",
        "liquidityNet": "109441723946229",
        "tickIdx": "199490"
      },
      {
        "liquidityGross": "2816471387298175",
        "liquidityNet": "-2816471387297651",
        "tickIdx": "199500"
      },
      {
        "liquidityGross": "835",
        "liquidityNet": "371",
        "tickIdx": "199520"
      },
      {
        "liquidityGross": "7049110291627469",
        "liquidityNet": "-7049110270089969",
        "tickIdx": "199540"
      },
      {
        "liquidityGross": "74496991922150",
        "liquidityNet": "74496991922150",
        "tickIdx": "199550"
      },
      {
        "liquidityGross": "1616020322757826",
        "liquidityNet": "1616020322756620",
        "tickIdx": "199560"
      },
      {
        "liquidityGross": "6114792526958",
        "liquidityNet": "6114792526958",
        "tickIdx": "199570"
      },
      {
        "liquidityGross": "1629468539025052",
        "liquidityNet": "-1629468539024590",
        "tickIdx": "199580"
      },
      {
        "liquidityGross": "1299528205655287",
        "liquidityNet": "-1177136826231823",
        "tickIdx": "199590"
      },
      {
        "liquidityGross": "10779755",
        "liquidityNet": "-10779293",
        "tickIdx": "199600"
      },
      {
        "liquidityGross": "809820099028192",
        "liquidityNet": "-809820099028192",
        "tickIdx": "199610"
      },
      {
        "liquidityGross": "304515142969478",
        "liquidityNet": "-304515142969016",
        "tickIdx": "199620"
      },
      {
        "liquidityGross": "533087168979636",
        "liquidityNet": "513032659291062",
        "tickIdx": "199630"
      },
      {
        "liquidityGross": "231",
        "liquidityNet": "-231",
        "tickIdx": "199640"
      },
      {
        "liquidityGross": "71560570730",
        "liquidityNet": "71560570730",
        "tickIdx": "199650"
      },
      {
        "liquidityGross": "231",
        "liquidityNet": "-231",
        "tickIdx": "199660"
      },
      {
        "liquidityGross": "214559524421144",
        "liquidityNet": "26101660793654",
        "tickIdx": "199680"
      },
      {
        "liquidityGross": "191701986657523",
        "liquidityNet": "191701986657523",
        "tickIdx": "199690"
      },
      {
        "liquidityGross": "425879693738649",
        "liquidityNet": "425879693738647",
        "tickIdx": "199700"
      },
      {
        "liquidityGross": "1442175329605960",
        "liquidityNet": "-1394522275932262",
        "tickIdx": "199710"
      },
      {
        "liquidityGross": "783773814508555",
        "liquidityNet": "783773814508095",
        "tickIdx": "199720"
      },
      {
        "liquidityGross": "14403264449810",
        "liquidityNet": "14403264449810",
        "tickIdx": "199730"
      },
      {
        "liquidityGross": "675",
        "liquidityNet": "215",
        "tickIdx": "199740"
      },
      {
        "liquidityGross": "338386054535499",
        "liquidityNet": "338386054535499",
        "tickIdx": "199750"
      },
      {
        "liquidityGross": "14403",
        "liquidityNet": "14403",
        "tickIdx": "199760"
      },
      {
        "liquidityGross": "486562890470664",
        "liquidityNet": "-2203656830826",
        "tickIdx": "199770"
      },
      {
        "liquidityGross": "306411773320982",
        "liquidityNet": "-306411751523488",
        "tickIdx": "199780"
      },
      {
        "liquidityGross": "142660157354006",
        "liquidityNet": "-142660157354006",
        "tickIdx": "199790"
      },
      {
        "liquidityGross": "9774472230221",
        "liquidityNet": "9774472201415",
        "tickIdx": "199800"
      },
      {
        "liquidityGross": "19841324105038",
        "liquidityNet": "19841324105038",
        "tickIdx": "199810"
      },
      {
        "liquidityGross": "134088442708624035",
        "liquidityNet": "132412111753933335",
        "tickIdx": "199820"
      },
      {
        "liquidityGross": "33999529151136",
        "liquidityNet": "24648158712216",
        "tickIdx": "199830"
      },
      {
        "liquidityGross": "10909651",
        "liquidityNet": "-10909651",
        "tickIdx": "199840"
      },
      {
        "liquidityGross": "8754807325732",
        "liquidityNet": "-8754785440878",
        "tickIdx": "199860"
      },
      {
        "liquidityGross": "132496772429688635",
        "liquidityNet": "-132496412790011197",
        "tickIdx": "199870"
      },
      {
        "liquidityGross": "1",
        "liquidityNet": "1",
        "tickIdx": "199880"
      },
      {
        "liquidityGross": "5524425706845",
        "liquidityNet": "-5524425706845",
        "tickIdx": "199890"
      },
      {
        "liquidityGross": "21906760",
        "liquidityNet": "21906",
        "tickIdx": "199900"
      },
      {
        "liquidityGross": "123704697866143",
        "liquidityNet": "123704697866143",
        "tickIdx": "199910"
      },
      {
        "liquidityGross": "624903395301679",
        "liquidityNet": "251553959107603",
        "tickIdx": "199920"
      },
      {
        "liquidityGross": "711088224021437",
        "liquidityNet": "-711088224021437",
        "tickIdx": "199930"
      },
      {
        "liquidityGross": "44749424061104",
        "liquidityNet": "-44749402088538",
        "tickIdx": "199940"
      },
      {
        "liquidityGross": "2549242597872279",
        "liquidityNet": "1788833624868295",
        "tickIdx": "199950"
      },
      {
        "liquidityGross": "646985406481961",
        "liquidityNet": "646985384531357",
        "tickIdx": "199960"
      },
      {
        "liquidityGross": "1110724847423185",
        "liquidityNet": "1110724847423183",
        "tickIdx": "199970"
      },
      {
        "liquidityGross": "21994559",
        "liquidityNet": "21993",
        "tickIdx": "199980"
      },
      {
        "liquidityGross": "1110724847423184",
        "liquidityNet": "-1110724847423184",
        "tickIdx": "199990"
      },
      {
        "liquidityGross": "37495929591945",
        "liquidityNet": "-5672632077963",
        "tickIdx": "200000"
      },
      {
        "liquidityGross": "227547268231657",
        "liquidityNet": "-227365776986897",
        "tickIdx": "200020"
      },
      {
        "liquidityGross": "90734592066",
        "liquidityNet": "-90734592066",
        "tickIdx": "200030"
      },
      {
        "liquidityGross": "22060638",
        "liquidityNet": "22060",
        "tickIdx": "200040"
      },
      {
        "liquidityGross": "104340034120545",
        "liquidityNet": "-104340034120545",
        "tickIdx": "200050"
      },
      {
        "liquidityGross": "22082709",
        "liquidityNet": "22081",
        "tickIdx": "200060"
      },
      {
        "liquidityGross": "74088806565007255",
        "liquidityNet": "-74088806542880349",
        "tickIdx": "200080"
      },
      {
        "liquidityGross": "22126917",
        "liquidityNet": "22127",
        "tickIdx": "200100"
      },
      {
        "liquidityGross": "7231564215441",
        "liquidityNet": "-7231564215441",
        "tickIdx": "200110"
      },
      {
        "liquidityGross": "374045660143",
        "liquidityNet": "374023533237",
        "tickIdx": "200120"
      },
      {
        "liquidityGross": "22171212",
        "liquidityNet": "22170",
        "tickIdx": "200140"
      },
      {
        "liquidityGross": "2794350631607160463",
        "liquidityNet": "2794350631607160463",
        "tickIdx": "200150"
      },
      {
        "liquidityGross": "68558810900586",
        "liquidityNet": "-68558788127722",
        "tickIdx": "200160"
      },
      {
        "liquidityGross": "9719393930962",
        "liquidityNet": "-9719393930962",
        "tickIdx": "200170"
      },
      {
        "liquidityGross": "22215598",
        "liquidityNet": "22214",
        "tickIdx": "200180"
      },
      {
        "liquidityGross": "678211560703923",
        "liquidityNet": "490373193234813",
        "tickIdx": "200190"
      },
      {
        "liquidityGross": "26815885329175",
        "liquidityNet": "-19430643592743",
        "tickIdx": "200200"
      },
      {
        "liquidityGross": "2024401591704571",
        "liquidityNet": "2023873390418693",
        "tickIdx": "200210"
      },
      {
        "liquidityGross": "993201586805",
        "liquidityNet": "993179348993",
        "tickIdx": "200220"
      },
      {
        "liquidityGross": "48494646361",
        "liquidityNet": "-48494646361",
        "tickIdx": "200230"
      },
      {
        "liquidityGross": "3199154781305376",
        "liquidityNet": "-1138921463695256",
        "tickIdx": "200240"
      },
      {
        "liquidityGross": "1",
        "liquidityNet": "-1",
        "tickIdx": "200250"
      },
      {
        "liquidityGross": "5987940193978175",
        "liquidityNet": "1855609337378309",
        "tickIdx": "200260"
      },
      {
        "liquidityGross": "22326947",
        "liquidityNet": "22325",
        "tickIdx": "200280"
      },
      {
        "liquidityGross": "1529687658581818",
        "liquidityNet": "1529687658581818",
        "tickIdx": "200290"
      },
      {
        "liquidityGross": "1331011848970010",
        "liquidityNet": "-1326183962814802",
        "tickIdx": "200300"
      },
      {
        "liquidityGross": "82911523828031212",
        "liquidityNet": "-73312633701085292",
        "tickIdx": "200310"
      },
      {
        "liquidityGross": "134658639244763",
        "liquidityNet": "98574242997795",
        "tickIdx": "200320"
      },
      {
        "liquidityGross": "57632422830797",
        "liquidityNet": "-27372571432279",
        "tickIdx": "200330"
      },
      {
        "liquidityGross": "3447494752645078",
        "liquidityNet": "2742414754506642",
        "tickIdx": "200340"
      },
      {
        "liquidityGross": "15129925699259",
        "liquidityNet": "-15129925699259",
        "tickIdx": "200350"
      },
      {
        "liquidityGross": "135581911785292",
        "liquidityNet": "-135581911784848",
        "tickIdx": "200360"
      },
      {
        "liquidityGross": "3930566173970",
        "liquidityNet": "-3930566173970",
        "tickIdx": "200370"
      },
      {
        "liquidityGross": "93855006830354",
        "liquidityNet": "93855006829908",
        "tickIdx": "200380"
      },
      {
        "liquidityGross": "1053450507491257",
        "liquidityNet": "939688197585005",
        "tickIdx": "200390"
      },
      {
        "liquidityGross": "510838932108746",
        "liquidityNet": "-510838932108302",
        "tickIdx": "200400"
      },
      {
        "liquidityGross": "2692886071143387",
        "liquidityNet": "-166900033607989",
        "tickIdx": "200410"
      },
      {
        "liquidityGross": "122792561080834992",
        "liquidityNet": "122792561080834992",
        "tickIdx": "200420"
      },
      {
        "liquidityGross": "6957598323464",
        "liquidityNet": "6957598209700",
        "tickIdx": "200430"
      },
      {
        "liquidityGross": "122792561080834992",
        "liquidityNet": "-122792561080834992",
        "tickIdx": "200440"
      },
      {
        "liquidityGross": "14566093142379",
        "liquidityNet": "14566093142379",
        "tickIdx": "200450"
      },
      {
        "liquidityGross": "8483504068880673",
        "liquidityNet": "-8483475989980353",
        "tickIdx": "200460"
      },
      {
        "liquidityGross": "351337821359505",
        "liquidityNet": "351337821359505",
        "tickIdx": "200470"
      },
      {
        "liquidityGross": "16051775223170",
        "liquidityNet": "-16051775222728",
        "tickIdx": "200480"
      },
      {
        "liquidityGross": "351337821359505",
        "liquidityNet": "-351337821359505",
        "tickIdx": "200490"
      },
      {
        "liquidityGross": "442",
        "liquidityNet": "0",
        "tickIdx": "200500"
      },
      {
        "liquidityGross": "36513897146351",
        "liquidityNet": "36513897146351",
        "tickIdx": "200510"
      },
      {
        "liquidityGross": "45319483289349",
        "liquidityNet": "45319483288907",
        "tickIdx": "200520"
      },
      {
        "liquidityGross": "21173915535375",
        "liquidityNet": "21173915535375",
        "tickIdx": "200530"
      },
      {
        "liquidityGross": "441",
        "liquidityNet": "-1",
        "tickIdx": "200540"
      },
      {
        "liquidityGross": "16569591813438",
        "liquidityNet": "-8284070484708",
        "tickIdx": "200550"
      },
      {
        "liquidityGross": "527125566062894",
        "liquidityNet": "-437617793065844",
        "tickIdx": "200560"
      },
      {
        "liquidityGross": "1982310279884812",
        "liquidityNet": "-1982310279884812",
        "tickIdx": "200570"
      },
      {
        "liquidityGross": "30964057740244",
        "liquidityNet": "30330952715726",
        "tickIdx": "200580"
      },
      {
        "liquidityGross": "71570634873",
        "liquidityNet": "71570634871",
        "tickIdx": "200590"
      },
      {
        "liquidityGross": "68095114764569",
        "liquidityNet": "68095114764129",
        "tickIdx": "200600"
      },
      {
        "liquidityGross": "1860176650658",
        "liquidityNet": "-1836850799640",
        "tickIdx": "200610"
      },
      {
        "liquidityGross": "68095114764568",
        "liquidityNet": "-68095114764130",
        "tickIdx": "200620"
      },
      {
        "liquidityGross": "420589345654229",
        "liquidityNet": "-420589345654229",
        "tickIdx": "200630"
      },
      {
        "liquidityGross": "815702302406817",
        "liquidityNet": "815702302406377",
        "tickIdx": "200640"
      },
      {
        "liquidityGross": "812983526947267",
        "liquidityNet": "-812983526946829",
        "tickIdx": "200660"
      },
      {
        "liquidityGross": "2106182033520489",
        "liquidityNet": "-2106182033520489",
        "tickIdx": "200670"
      },
      {
        "liquidityGross": "106429529365359",
        "liquidityNet": "106429529364921",
        "tickIdx": "200680"
      },
      {
        "liquidityGross": "147027791046072",
        "liquidityNet": "46604641066742",
        "tickIdx": "200690"
      },
      {
        "liquidityGross": "581260565700165",
        "liquidityNet": "-336955780229879",
        "tickIdx": "200700"
      },
      {
        "liquidityGross": "97019630318513",
        "liquidityNet": "-96612801794301",
        "tickIdx": "200710"
      },
      {
        "liquidityGross": "3224647536832895",
        "liquidityNet": "328276238126211",
        "tickIdx": "200720"
      },
      {
        "liquidityGross": "366843669884575",
        "liquidityNet": "-361792868277015",
        "tickIdx": "200730"
      },
      {
        "liquidityGross": "1003623340657575",
        "liquidityNet": "756037507768727",
        "tickIdx": "200740"
      },
      {
        "liquidityGross": "437640355755909",
        "liquidityNet": "-437640355755909",
        "tickIdx": "200750"
      },
      {
        "liquidityGross": "491775100291664",
        "liquidityNet": "-392605036622820",
        "tickIdx": "200760"
      },
      {
        "liquidityGross": "2657161661807451",
        "liquidityNet": "2634090598022917",
        "tickIdx": "200770"
      },
      {
        "liquidityGross": "61170764862676",
        "liquidityNet": "-350592129264",
        "tickIdx": "200780"
      },
      {
        "liquidityGross": "2830257076538314",
        "liquidityNet": "-1965897650637536",
        "tickIdx": "200790"
      },
      {
        "liquidityGross": "22266573740998",
        "liquidityNet": "-4817758115102",
        "tickIdx": "200800"
      },
      {
        "liquidityGross": "1647919843839365",
        "liquidityNet": "539763155419907",
        "tickIdx": "200810"
      },
      {
        "liquidityGross": "58812993697256645",
        "liquidityNet": "-58632790214630591",
        "tickIdx": "200820"
      },
      {
        "liquidityGross": "1966145004214497",
        "liquidityNet": "-1857694841713043",
        "tickIdx": "200830"
      },
      {
        "liquidityGross": "5935929477015998",
        "liquidityNet": "5935929477015562",
        "tickIdx": "200840"
      },
      {
        "liquidityGross": "949032241194118",
        "liquidityNet": "844988115259702",
        "tickIdx": "200850"
      },
      {
        "liquidityGross": "7607337254229732040",
        "liquidityNet": "-7590074359994158930",
        "tickIdx": "200860"
      },
      {
        "liquidityGross": "1353587997990326790",
        "liquidityNet": "-1353585769302967830",
        "tickIdx": "200870"
      },
      {
        "liquidityGross": "6079826813262725",
        "liquidityNet": "-5807344029001399",
        "tickIdx": "200880"
      },
      {
        "liquidityGross": "160871579222557513",
        "liquidityNet": "160869487974152025",
        "tickIdx": "200890"
      },
      {
        "liquidityGross": "806",
        "liquidityNet": "372",
        "tickIdx": "200900"
      },
      {
        "liquidityGross": "160753138694254855",
        "liquidityNet": "-160753138694254855",
        "tickIdx": "200910"
      },
      {
        "liquidityGross": "124795786263918",
        "liquidityNet": "-122613609468868",
        "tickIdx": "200920"
      },
      {
        "liquidityGross": "5926908939545144",
        "liquidityNet": "-5491879378840760",
        "tickIdx": "200930"
      },
      {
        "liquidityGross": "1034480195732719",
        "liquidityNet": "-1034480172633653",
        "tickIdx": "200940"
      },
      {
        "liquidityGross": "156002102874199",
        "liquidityNet": "-126458672292395",
        "tickIdx": "200950"
      },
      {
        "liquidityGross": "92921501471031",
        "liquidityNet": "-92921478348857",
        "tickIdx": "200960"
      },
      {
        "liquidityGross": "1358724166319069",
        "liquidityNet": "1356580988636911",
        "tickIdx": "200970"
      },
      {
        "liquidityGross": "2027366695954000",
        "liquidityNet": "1652324107541166",
        "tickIdx": "200980"
      },
      {
        "liquidityGross": "1371488871080303",
        "liquidityNet": "-1371488871080303",
        "tickIdx": "200990"
      },
      {
        "liquidityGross": "781772065160912",
        "liquidityNet": "702617002410770",
        "tickIdx": "201000"
      },
      {
        "liquidityGross": "4317699066346711",
        "liquidityNet": "4315586199005097",
        "tickIdx": "201010"
      },
      {
        "liquidityGross": "592323561734495",
        "liquidityNet": "582822049176087",
        "tickIdx": "201020"
      },
      {
        "liquidityGross": "6388054037658248",
        "liquidityNet": "-6048431382098680",
        "tickIdx": "201030"
      },
      {
        "liquidityGross": "11228721486804618",
        "liquidityNet": "-3557979882117344",
        "tickIdx": "201040"
      },
      {
        "liquidityGross": "200270649597064",
        "liquidityNet": "80880024376906",
        "tickIdx": "201050"
      },
      {
        "liquidityGross": "39050849077330",
        "liquidityNet": "-38042042688804",
        "tickIdx": "201060"
      },
      {
        "liquidityGross": "615318939307057",
        "liquidityNet": "-269000739269873",
        "tickIdx": "201070"
      },
      {
        "liquidityGross": "135046345652111",
        "liquidityNet": "68956702828135",
        "tickIdx": "201080"
      },
      {
        "liquidityGross": "3336142887562818",
        "liquidityNet": "2694658607802594",
        "tickIdx": "201090"
      },
      {
        "liquidityGross": "3141023900646091",
        "liquidityNet": "-3055376945953063",
        "tickIdx": "201100"
      },
      {
        "liquidityGross": "51999305383353",
        "liquidityNet": "-34814551861353",
        "tickIdx": "201110"
      },
      {
        "liquidityGross": "388182900103967",
        "liquidityNet": "370998123320649",
        "tickIdx": "201120"
      },
      {
        "liquidityGross": "77307541856785",
        "liquidityNet": "-77307541856785",
        "tickIdx": "201130"
      },
      {
        "liquidityGross": "355906665034707",
        "liquidityNet": "-355906641703505",
        "tickIdx": "201140"
      },
      {
        "liquidityGross": "14394526573216294",
        "liquidityNet": "3185240372147028",
        "tickIdx": "201150"
      },
      {
        "liquidityGross": "60677511541961",
        "liquidityNet": "60677488234077",
        "tickIdx": "201160"
      },
      {
        "liquidityGross": "51820593366424",
        "liquidityNet": "48817847062364",
        "tickIdx": "201170"
      },
      {
        "liquidityGross": "62052168023219",
        "liquidityNet": "-59302831729477",
        "tickIdx": "201180"
      },
      {
        "liquidityGross": "1483277048793441",
        "liquidityNet": "1483277048793441",
        "tickIdx": "201190"
      },
      {
        "liquidityGross": "54109862781336",
        "liquidityNet": "51360526510960",
        "tickIdx": "201200"
      },
      {
        "liquidityGross": "1946170645085782",
        "liquidityNet": "1797141477859690",
        "tickIdx": "201210"
      },
      {
        "liquidityGross": "6480535467861053",
        "liquidityNet": "6480535444483143",
        "tickIdx": "201220"
      },
      {
        "liquidityGross": "1916872620130308",
        "liquidityNet": "-1771675766918566",
        "tickIdx": "201230"
      },
      {
        "liquidityGross": "7404889069607928",
        "liquidityNet": "-5661652208603854",
        "tickIdx": "201240"
      },
      {
        "liquidityGross": "8660433954155163",
        "liquidityNet": "-8157871729957143",
        "tickIdx": "201250"
      },
      {
        "liquidityGross": "2539108155329676",
        "liquidityNet": "1130215229414602",
        "tickIdx": "201260"
      },
      {
        "liquidityGross": "10095617936591774",
        "liquidityNet": "9752955799890292",
        "tickIdx": "201270"
      },
      {
        "liquidityGross": "10569641727709273",
        "liquidityNet": "-10249529180754647",
        "tickIdx": "201280"
      },
      {
        "liquidityGross": "1442669581710313",
        "liquidityNet": "-1373590060285777",
        "tickIdx": "201290"
      },
      {
        "liquidityGross": "160525438828030",
        "liquidityNet": "-160525415309438",
        "tickIdx": "201300"
      },
      {
        "liquidityGross": "1095681776003922",
        "liquidityNet": "477712266859626",
        "tickIdx": "201310"
      },
      {
        "liquidityGross": "68960865886731",
        "liquidityNet": "66974483738175",
        "tickIdx": "201320"
      },
      {
        "liquidityGross": "117320418846232",
        "liquidityNet": "-117320418846232",
        "tickIdx": "201330"
      },
      {
        "liquidityGross": "67777611265632",
        "liquidityNet": "-67777587699958",
        "tickIdx": "201340"
      },
      {
        "liquidityGross": "4011809697059212",
        "liquidityNet": "4011809697059212",
        "tickIdx": "201350"
      },
      {
        "liquidityGross": "16148728818678420",
        "liquidityNet": "-9321659891831588",
        "tickIdx": "201360"
      },
      {
        "liquidityGross": "1596025568956328",
        "liquidityNet": "41423489074448",
        "tickIdx": "201370"
      },
      {
        "liquidityGross": "1549989008658237",
        "liquidityNet": "-1539253825653133",
        "tickIdx": "201380"
      },
      {
        "liquidityGross": "67824229776976",
        "liquidityNet": "-4990386824878",
        "tickIdx": "201390"
      },
      {
        "liquidityGross": "1905761853128514",
        "liquidityNet": "1886785011478798",
        "tickIdx": "201400"
      },
      {
        "liquidityGross": "3091556223440611",
        "liquidityNet": "3028722380488513",
        "tickIdx": "201410"
      },
      {
        "liquidityGross": "2573664387285085",
        "liquidityNet": "-1234960806301203",
        "tickIdx": "201420"
      },
      {
        "liquidityGross": "3729169504894378",
        "liquidityNet": "-3729169504894378",
        "tickIdx": "201430"
      },
      {
        "liquidityGross": "823159501087986",
        "liquidityNet": "-733314569528550",
        "tickIdx": "201440"
      },
      {
        "liquidityGross": "14367994359976201",
        "liquidityNet": "-13547674333618117",
        "tickIdx": "201450"
      },
      {
        "liquidityGross": "2166617532252280",
        "liquidityNet": "-650907398108510",
        "tickIdx": "201460"
      },
      {
        "liquidityGross": "36119998222938",
        "liquidityNet": "-16827750181546",
        "tickIdx": "201470"
      },
      {
        "liquidityGross": "1942744046868201",
        "liquidityNet": "-1938831683495901",
        "tickIdx": "201480"
      },
      {
        "liquidityGross": "123908795710071",
        "liquidityNet": "104616547668679",
        "tickIdx": "201490"
      },
      {
        "liquidityGross": "3991888933632115",
        "liquidityNet": "-3855572938769733",
        "tickIdx": "201500"
      },
      {
        "liquidityGross": "727076815925031",
        "liquidityNet": "-727076815925031",
        "tickIdx": "201510"
      },
      {
        "liquidityGross": "113477480720098",
        "liquidityNet": "-113477480720098",
        "tickIdx": "201520"
      },
      {
        "liquidityGross": "6858909678292141",
        "liquidityNet": "6824239110568007",
        "tickIdx": "201530"
      },
      {
        "liquidityGross": "32563202638530",
        "liquidityNet": "32563202638530",
        "tickIdx": "201550"
      },
      {
        "liquidityGross": "1114705087551475",
        "liquidityNet": "-1114705087551475",
        "tickIdx": "201560"
      },
      {
        "liquidityGross": "2164250889876547",
        "liquidityNet": "-2164250889876547",
        "tickIdx": "201570"
      },
      {
        "liquidityGross": "950545747548079",
        "liquidityNet": "885960592770969",
        "tickIdx": "201580"
      },
      {
        "liquidityGross": "2077209166935057",
        "liquidityNet": "-1029158315195265",
        "tickIdx": "201590"
      },
      {
        "liquidityGross": "369024484077202",
        "liquidityNet": "-369024484077202",
        "tickIdx": "201600"
      },
      {
        "liquidityGross": "109270333727788",
        "liquidityNet": "-101525303618758",
        "tickIdx": "201610"
      },
      {
        "liquidityGross": "4792164193",
        "liquidityNet": "4792164193",
        "tickIdx": "201620"
      },
      {
        "liquidityGross": "3872515054515",
        "liquidityNet": "-3872515054515",
        "tickIdx": "201630"
      },
      {
        "liquidityGross": "27586179889260568",
        "liquidityNet": "-27009485245212022",
        "tickIdx": "201640"
      },
      {
        "liquidityGross": "1534951463702752",
        "liquidityNet": "765361740583450",
        "tickIdx": "201650"
      },
      {
        "liquidityGross": "34603690541862",
        "liquidityNet": "-33390114239104",
        "tickIdx": "201660"
      },
      {
        "liquidityGross": "1527085319688934",
        "liquidityNet": "-775342162142592",
        "tickIdx": "201670"
      },
      {
        "liquidityGross": "301087128591181",
        "liquidityNet": "-198242218931975",
        "tickIdx": "201680"
      },
      {
        "liquidityGross": "160134104478378",
        "liquidityNet": "-105012970700080",
        "tickIdx": "201690"
      },
      {
        "liquidityGross": "23144534925279",
        "liquidityNet": "-23144534925279",
        "tickIdx": "201700"
      },
      {
        "liquidityGross": "100530336353123",
        "liquidityNet": "98614049897619",
        "tickIdx": "201710"
      },
      {
        "liquidityGross": "87969225024706",
        "liquidityNet": "-78809282445782",
        "tickIdx": "201720"
      },
      {
        "liquidityGross": "141302275465258",
        "liquidityNet": "128694428325432",
        "tickIdx": "201730"
      },
      {
        "liquidityGross": "141071003624038",
        "liquidityNet": "-140590409734278",
        "tickIdx": "201740"
      },
      {
        "liquidityGross": "31477761011204",
        "liquidityNet": "-31477761011204",
        "tickIdx": "201750"
      },
      {
        "liquidityGross": "75848803157030",
        "liquidityNet": "55594628576024",
        "tickIdx": "201760"
      },
      {
        "liquidityGross": "8544138",
        "liquidityNet": "-8544138",
        "tickIdx": "201770"
      },
      {
        "liquidityGross": "44417949198275",
        "liquidityNet": "-27283555976315",
        "tickIdx": "201780"
      },
      {
        "liquidityGross": "91599811640864",
        "liquidityNet": "-91599811640864",
        "tickIdx": "201790"
      },
      {
        "liquidityGross": "9319971869154",
        "liquidityNet": "-9319971869154",
        "tickIdx": "201800"
      },
      {
        "liquidityGross": "122846685345363",
        "liquidityNet": "122846685345363",
        "tickIdx": "201810"
      },
      {
        "liquidityGross": "32789672924553",
        "liquidityNet": "-32789672023349",
        "tickIdx": "201820"
      },
      {
        "liquidityGross": "122847048153463",
        "liquidityNet": "-122846322537263",
        "tickIdx": "201830"
      },
      {
        "liquidityGross": "3293926376305",
        "liquidityNet": "3293926376305",
        "tickIdx": "201840"
      },
      {
        "liquidityGross": "75303493831926",
        "liquidityNet": "-75303493831926",
        "tickIdx": "201850"
      },
      {
        "liquidityGross": "98747458896728",
        "liquidityNet": "98747458896728",
        "tickIdx": "201860"
      },
      {
        "liquidityGross": "27576606725510",
        "liquidityNet": "27576606725510",
        "tickIdx": "201880"
      },
      {
        "liquidityGross": "304140118800551",
        "liquidityNet": "304140118800551",
        "tickIdx": "201890"
      },
      {
        "liquidityGross": "443297704764811",
        "liquidityNet": "-381517256167665",
        "tickIdx": "201900"
      },
      {
        "liquidityGross": "3008549830132364",
        "liquidityNet": "1557511096631416",
        "tickIdx": "201910"
      },
      {
        "liquidityGross": "1490752855615372",
        "liquidityNet": "1490752855615372",
        "tickIdx": "201920"
      },
      {
        "liquidityGross": "1656956536187030",
        "liquidityNet": "-1328000075072396",
        "tickIdx": "201930"
      },
      {
        "liquidityGross": "2800763799648160687",
        "liquidityNet": "-2796183445280093413",
        "tickIdx": "201940"
      },
      {
        "liquidityGross": "1105860665511954",
        "liquidityNet": "8772087391684",
        "tickIdx": "201950"
      },
      {
        "liquidityGross": "2022334482141697",
        "liquidityNet": "-2022334482141697",
        "tickIdx": "201960"
      },
      {
        "liquidityGross": "29467823437210",
        "liquidityNet": "-29467823437210",
        "tickIdx": "201980"
      },
      {
        "liquidityGross": "918253170159524",
        "liquidityNet": "-918253170159524",
        "tickIdx": "201990"
      },
      {
        "liquidityGross": "1443751488965052",
        "liquidityNet": "1293604400121640",
        "tickIdx": "202000"
      },
      {
        "liquidityGross": "371214130284398",
        "liquidityNet": "371214130284398",
        "tickIdx": "202020"
      },
      {
        "liquidityGross": "88359034719805",
        "liquidityNet": "-88359034719805",
        "tickIdx": "202030"
      },
      {
        "liquidityGross": "1165747826076894",
        "liquidityNet": "-1165747826076894",
        "tickIdx": "202040"
      },
      {
        "liquidityGross": "2112036435530125",
        "liquidityNet": "1979596997391265",
        "tickIdx": "202050"
      },
      {
        "liquidityGross": "1746117654131131",
        "liquidityNet": "385341402660873",
        "tickIdx": "202060"
      },
      {
        "liquidityGross": "1463745746507204",
        "liquidityNet": "1256094250164338",
        "tickIdx": "202070"
      },
      {
        "liquidityGross": "1289174392026452",
        "liquidityNet": "-1289117518890826",
        "tickIdx": "202080"
      },
      {
        "liquidityGross": "1044740064248498",
        "liquidityNet": "899518794073598",
        "tickIdx": "202090"
      },
      {
        "liquidityGross": "776984662305219",
        "liquidityNet": "34556401736423",
        "tickIdx": "202100"
      },
      {
        "liquidityGross": "1964611489752263",
        "liquidityNet": "-207614746038809",
        "tickIdx": "202110"
      },
      {
        "liquidityGross": "456089752235215",
        "liquidityNet": "-456089752235215",
        "tickIdx": "202120"
      },
      {
        "liquidityGross": "887158386419342",
        "liquidityNet": "-873719823639174",
        "tickIdx": "202130"
      },
      {
        "liquidityGross": "1217222937037888",
        "liquidityNet": "1203784374257720",
        "tickIdx": "202150"
      },
      {
        "liquidityGross": "1460098731480152",
        "liquidityNet": "1460098731480152",
        "tickIdx": "202160"
      },
      {
        "liquidityGross": "1215487224091290",
        "liquidityNet": "-1205520087204318",
        "tickIdx": "202170"
      },
      {
        "liquidityGross": "2272610849089484",
        "liquidityNet": "-666672863230672",
        "tickIdx": "202180"
      },
      {
        "liquidityGross": "794877097843566",
        "liquidityNet": "-721127241915418",
        "tickIdx": "202190"
      },
      {
        "liquidityGross": "2841271059224749",
        "liquidityNet": "2841271059224749",
        "tickIdx": "202200"
      },
      {
        "liquidityGross": "20804432915779",
        "liquidityNet": "20804432915779",
        "tickIdx": "202210"
      },
      {
        "liquidityGross": "2886116005280854",
        "liquidityNet": "-2886116005280854",
        "tickIdx": "202220"
      },
      {
        "liquidityGross": "3898705337956426",
        "liquidityNet": "3890419816627696",
        "tickIdx": "202230"
      },
      {
        "liquidityGross": "5341527419320384",
        "liquidityNet": "4846086921315790",
        "tickIdx": "202240"
      },
      {
        "liquidityGross": "1861249977247649",
        "liquidityNet": "-1845831689201499",
        "tickIdx": "202250"
      },
      {
        "liquidityGross": "98747458896728",
        "liquidityNet": "-98747458896728",
        "tickIdx": "202260"
      },
      {
        "liquidityGross": "948515720825901",
        "liquidityNet": "948515720825901",
        "tickIdx": "202270"
      },
      {
        "liquidityGross": "651926017759613",
        "liquidityNet": "-606932490709277",
        "tickIdx": "202280"
      },
      {
        "liquidityGross": "1040026172519131",
        "liquidityNet": "-1040026172519131",
        "tickIdx": "202290"
      },
      {
        "liquidityGross": "107281811783408",
        "liquidityNet": "-84431771944744",
        "tickIdx": "202300"
      },
      {
        "liquidityGross": "1077699200074935",
        "liquidityNet": "1016658362362261",
        "tickIdx": "202310"
      },
      {
        "liquidityGross": "46314553633318",
        "liquidityNet": "1485425173462",
        "tickIdx": "202320"
      },
      {
        "liquidityGross": "646303867755704",
        "liquidityNet": "-530380071325622",
        "tickIdx": "202330"
      },
      {
        "liquidityGross": "17882457470532002",
        "liquidityNet": "17826169961373778",
        "tickIdx": "202340"
      },
      {
        "liquidityGross": "4544487085982590",
        "liquidityNet": "-4054038137938264",
        "tickIdx": "202350"
      },
      {
        "liquidityGross": "1108466582107019",
        "liquidityNet": "37688888135713",
        "tickIdx": "202360"
      },
      {
        "liquidityGross": "17661869039973",
        "liquidityNet": "-5181401192827",
        "tickIdx": "202370"
      },
      {
        "liquidityGross": "297250035413589",
        "liquidityNet": "275850618898897",
        "tickIdx": "202380"
      },
      {
        "liquidityGross": "202239912360275",
        "liquidityNet": "-202239912360275",
        "tickIdx": "202390"
      },
      {
        "liquidityGross": "1045062518975539",
        "liquidityNet": "11907509894361",
        "tickIdx": "202400"
      },
      {
        "liquidityGross": "291709804725254",
        "liquidityNet": "82947004278596",
        "tickIdx": "202410"
      },
      {
        "liquidityGross": "581553348955679",
        "liquidityNet": "-475416679914221",
        "tickIdx": "202420"
      },
      {
        "liquidityGross": "274050207472783",
        "liquidityNet": "-231958528444287",
        "tickIdx": "202430"
      },
      {
        "liquidityGross": "49749484",
        "liquidityNet": "49749484",
        "tickIdx": "202440"
      },
      {
        "liquidityGross": "208373734671811",
        "liquidityNet": "-208373734671811",
        "tickIdx": "202450"
      },
      {
        "liquidityGross": "179833682186913",
        "liquidityNet": "-6586339534015",
        "tickIdx": "202470"
      },
      {
        "liquidityGross": "830480927295548",
        "liquidityNet": "-830480927295548",
        "tickIdx": "202480"
      },
      {
        "liquidityGross": "294207921674184",
        "liquidityNet": "294207921674184",
        "tickIdx": "202490"
      },
      {
        "liquidityGross": "17957417719219246",
        "liquidityNet": "-17957417719219246",
        "tickIdx": "202510"
      },
      {
        "liquidityGross": "4443296792250825",
        "liquidityNet": "-4385939184862505",
        "tickIdx": "202520"
      },
      {
        "liquidityGross": "4903568332927367",
        "liquidityNet": "576687292514407",
        "tickIdx": "202540"
      },
      {
        "liquidityGross": "162102198680786",
        "liquidityNet": "-162102198680786",
        "tickIdx": "202550"
      },
      {
        "liquidityGross": "4400484934565610",
        "liquidityNet": "4400484934565610",
        "tickIdx": "202560"
      },
      {
        "liquidityGross": "4628853452881895",
        "liquidityNet": "-4172116416249325",
        "tickIdx": "202570"
      },
      {
        "liquidityGross": "13305292578630",
        "liquidityNet": "13305292578630",
        "tickIdx": "202580"
      },
      {
        "liquidityGross": "382961297317448",
        "liquidityNet": "-376755609839576",
        "tickIdx": "202590"
      },
      {
        "liquidityGross": "13305292578630",
        "liquidityNet": "-13305292578630",
        "tickIdx": "202600"
      },
      {
        "liquidityGross": "991459414856411",
        "liquidityNet": "-991459414856411",
        "tickIdx": "202610"
      },
      {
        "liquidityGross": "462916170411318",
        "liquidityNet": "17868651158522",
        "tickIdx": "202620"
      },
      {
        "liquidityGross": "29245318865145",
        "liquidityNet": "-29245318865145",
        "tickIdx": "202630"
      },
      {
        "liquidityGross": "240392410784920",
        "liquidityNet": "-240392410784920",
        "tickIdx": "202640"
      },
      {
        "liquidityGross": "10218639059771",
        "liquidityNet": "-2575858156917",
        "tickIdx": "202650"
      },
      {
        "liquidityGross": "7883035712938",
        "liquidityNet": "586789738158",
        "tickIdx": "202660"
      },
      {
        "liquidityGross": "4987553801879075",
        "liquidityNet": "-4220267505664283",
        "tickIdx": "202670"
      },
      {
        "liquidityGross": "4234912725548",
        "liquidityNet": "-4234912725548",
        "tickIdx": "202680"
      },
      {
        "liquidityGross": "209975311487342",
        "liquidityNet": "-209975311487342",
        "tickIdx": "202690"
      },
      {
        "liquidityGross": "4023070355157006",
        "liquidityNet": "3912691503080106",
        "tickIdx": "202700"
      },
      {
        "liquidityGross": "7010548566953618",
        "liquidityNet": "-3045642408061188",
        "tickIdx": "202710"
      },
      {
        "liquidityGross": "405576329100360",
        "liquidityNet": "405576329100360",
        "tickIdx": "202720"
      },
      {
        "liquidityGross": "96120806807801145",
        "liquidityNet": "92075174683119781",
        "tickIdx": "202730"
      },
      {
        "liquidityGross": "93413072204211757",
        "liquidityNet": "-93412511999162297",
        "tickIdx": "202750"
      },
      {
        "liquidityGross": "405576329100360",
        "liquidityNet": "-405576329100360",
        "tickIdx": "202760"
      },
      {
        "liquidityGross": "635875721240268",
        "liquidityNet": "-635875721240268",
        "tickIdx": "202770"
      },
      {
        "liquidityGross": "70088886627848",
        "liquidityNet": "39367994112418",
        "tickIdx": "202780"
      },
      {
        "liquidityGross": "87469033315472",
        "liquidityNet": "86543163008874",
        "tickIdx": "202790"
      },
      {
        "liquidityGross": "936190454983278",
        "liquidityNet": "534741969435138",
        "tickIdx": "202800"
      },
      {
        "liquidityGross": "223778444452698",
        "liquidityNet": "49766248128352",
        "tickIdx": "202810"
      },
      {
        "liquidityGross": "891270577696723",
        "liquidityNet": "-853206539302727",
        "tickIdx": "202820"
      },
      {
        "liquidityGross": "314569659716951",
        "liquidityNet": "314569659716951",
        "tickIdx": "202830"
      },
      {
        "liquidityGross": "19032019196998",
        "liquidityNet": "-19032019196998",
        "tickIdx": "202840"
      },
      {
        "liquidityGross": "330661109678975",
        "liquidityNet": "-298478209754927",
        "tickIdx": "202850"
      },
      {
        "liquidityGross": "4583598845325014",
        "liquidityNet": "-4400291569925088",
        "tickIdx": "202860"
      },
      {
        "liquidityGross": "1802570040104479",
        "liquidityNet": "-1790722478715099",
        "tickIdx": "202870"
      },
      {
        "liquidityGross": "102144853689538",
        "liquidityNet": "86481145519812",
        "tickIdx": "202880"
      },
      {
        "liquidityGross": "159707892442444",
        "liquidityNet": "-159707892442444",
        "tickIdx": "202890"
      },
      {
        "liquidityGross": "124779696182196",
        "liquidityNet": "-124779696182196",
        "tickIdx": "202900"
      },
      {
        "liquidityGross": "1559501623180",
        "liquidityNet": "1559500721976",
        "tickIdx": "202920"
      },
      {
        "liquidityGross": "44770824606692",
        "liquidityNet": "-44770824606692",
        "tickIdx": "202960"
      },
      {
        "liquidityGross": "89233030144128",
        "liquidityNet": "-87133314853926",
        "tickIdx": "202970"
      },
      {
        "liquidityGross": "17566865042867",
        "liquidityNet": "-17566865042867",
        "tickIdx": "202980"
      },
      {
        "liquidityGross": "53120179115163",
        "liquidityNet": "-53120179115163",
        "tickIdx": "202990"
      },
      {
        "liquidityGross": "7181254743610",
        "liquidityNet": "-7181254743610",
        "tickIdx": "203000"
      },
      {
        "liquidityGross": "8407139530500",
        "liquidityNet": "-8407139530500",
        "tickIdx": "203020"
      },
      {
        "liquidityGross": "101785000042774",
        "liquidityNet": "-101785000042774",
        "tickIdx": "203040"
      },
      {
        "liquidityGross": "598080051491788",
        "liquidityNet": "598080051491788",
        "tickIdx": "203050"
      },
      {
        "liquidityGross": "6088612861499",
        "liquidityNet": "-6088612861499",
        "tickIdx": "203060"
      },
      {
        "liquidityGross": "28277919904324",
        "liquidityNet": "-28277919904324",
        "tickIdx": "203100"
      },
      {
        "liquidityGross": "5231689491892",
        "liquidityNet": "-5231689491892",
        "tickIdx": "203120"
      },
      {
        "liquidityGross": "56979434141331",
        "liquidityNet": "-32865497418105",
        "tickIdx": "203130"
      },
      {
        "liquidityGross": "98726939861609",
        "liquidityNet": "-98726939861609",
        "tickIdx": "203140"
      },
      {
        "liquidityGross": "41481054197019",
        "liquidityNet": "-41481054197019",
        "tickIdx": "203150"
      },
      {
        "liquidityGross": "15336904627316",
        "liquidityNet": "-15336904627316",
        "tickIdx": "203160"
      },
      {
        "liquidityGross": "24388615498285",
        "liquidityNet": "-24388615498285",
        "tickIdx": "203170"
      },
      {
        "liquidityGross": "2316828376572938",
        "liquidityNet": "-2290011731112124",
        "tickIdx": "203180"
      },
      {
        "liquidityGross": "62891568007528779",
        "liquidityNet": "-47844619771628543",
        "tickIdx": "203190"
      },
      {
        "liquidityGross": "584738576398555",
        "liquidityNet": "-584738576398555",
        "tickIdx": "203200"
      },
      {
        "liquidityGross": "7619473815173",
        "liquidityNet": "-7619473815173",
        "tickIdx": "203220"
      },
      {
        "liquidityGross": "636124044933302",
        "liquidityNet": "-636124044933302",
        "tickIdx": "203260"
      },
      {
        "liquidityGross": "1880268460956411",
        "liquidityNet": "-1880268460956411",
        "tickIdx": "203270"
      },
      {
        "liquidityGross": "173667836620054",
        "liquidityNet": "-173667836620054",
        "tickIdx": "203290"
      },
      {
        "liquidityGross": "17732822804684",
        "liquidityNet": "-17732822804684",
        "tickIdx": "203320"
      },
      {
        "liquidityGross": "42823465704219",
        "liquidityNet": "-42823465704219",
        "tickIdx": "203330"
      },
      {
        "liquidityGross": "19957448935044",
        "liquidityNet": "-19957448935044",
        "tickIdx": "203340"
      },
      {
        "liquidityGross": "1590441756031",
        "liquidityNet": "-1590441756031",
        "tickIdx": "203360"
      },
      {
        "liquidityGross": "22560478618918",
        "liquidityNet": "-22560478618918",
        "tickIdx": "203390"
      },
      {
        "liquidityGross": "507090483264063",
        "liquidityNet": "-507090483264063",
        "tickIdx": "203400"
      },
      {
        "liquidityGross": "948347949819961",
        "liquidityNet": "-948347949819961",
        "tickIdx": "203450"
      },
      {
        "liquidityGross": "4401764424180",
        "liquidityNet": "-4401764424180",
        "tickIdx": "203460"
      },
      {
        "liquidityGross": "9287401875839",
        "liquidityNet": "-9287401875839",
        "tickIdx": "203470"
      },
      {
        "liquidityGross": "66885192117",
        "liquidityNet": "-66885192117",
        "tickIdx": "203490"
      },
      {
        "liquidityGross": "6560098309762",
        "liquidityNet": "-6560098309762",
        "tickIdx": "203520"
      },
      {
        "liquidityGross": "10738102070491345",
        "liquidityNet": "-10736192110597097",
        "tickIdx": "203530"
      },
      {
        "liquidityGross": "73502870437",
        "liquidityNet": "-73502870437",
        "tickIdx": "203540"
      },
      {
        "liquidityGross": "188138191644572",
        "liquidityNet": "-146194548038870",
        "tickIdx": "203550"
      },
      {
        "liquidityGross": "41148245238",
        "liquidityNet": "41148245238",
        "tickIdx": "203600"
      },
      {
        "liquidityGross": "12644016052739932",
        "liquidityNet": "-12644016052739932",
        "tickIdx": "203610"
      },
      {
        "liquidityGross": "18824353338452",
        "liquidityNet": "-18824353338452",
        "tickIdx": "203620"
      },
      {
        "liquidityGross": "13321068754020790",
        "liquidityNet": "-13321068754020790",
        "tickIdx": "203630"
      },
      {
        "liquidityGross": "598080051491778",
        "liquidityNet": "-598080051491778",
        "tickIdx": "203640"
      },
      {
        "liquidityGross": "14319313250054",
        "liquidityNet": "-14319313250054",
        "tickIdx": "203680"
      },
      {
        "liquidityGross": "29406113433896",
        "liquidityNet": "-29406113433896",
        "tickIdx": "203700"
      },
      {
        "liquidityGross": "29187215029237",
        "liquidityNet": "-29187215029237",
        "tickIdx": "203720"
      },
      {
        "liquidityGross": "25808506038260",
        "liquidityNet": "-25808506038260",
        "tickIdx": "203740"
      },
      {
        "liquidityGross": "37593079364642",
        "liquidityNet": "37593079364642",
        "tickIdx": "203780"
      },
      {
        "liquidityGross": "62958230984033",
        "liquidityNet": "-62958230984033",
        "tickIdx": "203790"
      },
      {
        "liquidityGross": "410514541597419",
        "liquidityNet": "-410514541597419",
        "tickIdx": "203810"
      },
      {
        "liquidityGross": "4459378691580",
        "liquidityNet": "-4459378691580",
        "tickIdx": "203820"
      },
      {
        "liquidityGross": "190075317894",
        "liquidityNet": "-190075317894",
        "tickIdx": "203840"
      },
      {
        "liquidityGross": "2563707068368625",
        "liquidityNet": "-2563707068368625",
        "tickIdx": "203860"
      },
      {
        "liquidityGross": "4068284447161069",
        "liquidityNet": "-4039297443936569",
        "tickIdx": "203870"
      },
      {
        "liquidityGross": "23015602873053776",
        "liquidityNet": "-22760578394485304",
        "tickIdx": "203880"
      },
      {
        "liquidityGross": "21978177723817",
        "liquidityNet": "6489986004223",
        "tickIdx": "203890"
      },
      {
        "liquidityGross": "7500972438780182",
        "liquidityNet": "-7500972438780182",
        "tickIdx": "203920"
      },
      {
        "liquidityGross": "33906611355",
        "liquidityNet": "33906611355",
        "tickIdx": "203950"
      },
      {
        "liquidityGross": "1184074883211",
        "liquidityNet": "1184074883211",
        "tickIdx": "203960"
      },
      {
        "liquidityGross": "15213000014794",
        "liquidityNet": "15213000014794",
        "tickIdx": "203990"
      },
      {
        "liquidityGross": "2721055168095",
        "liquidityNet": "-2721055168095",
        "tickIdx": "204020"
      },
      {
        "liquidityGross": "65359217603",
        "liquidityNet": "-65359217603",
        "tickIdx": "204030"
      },
      {
        "liquidityGross": "7168655126983",
        "liquidityNet": "-7168655126983",
        "tickIdx": "204040"
      },
      {
        "liquidityGross": "224653000969518",
        "liquidityNet": "-224653000969518",
        "tickIdx": "204050"
      },
      {
        "liquidityGross": "2610495386081331",
        "liquidityNet": "-2610495386081331",
        "tickIdx": "204060"
      },
      {
        "liquidityGross": "23405696260743",
        "liquidityNet": "-23405696260743",
        "tickIdx": "204070"
      },
      {
        "liquidityGross": "44749132440304",
        "liquidityNet": "44749132440304",
        "tickIdx": "204080"
      },
      {
        "liquidityGross": "564499432987176",
        "liquidityNet": "366010862813302",
        "tickIdx": "204090"
      },
      {
        "liquidityGross": "15580378389654",
        "liquidityNet": "-15580378389654",
        "tickIdx": "204100"
      },
      {
        "liquidityGross": "3223954871333",
        "liquidityNet": "-3223954871333",
        "tickIdx": "204120"
      },
      {
        "liquidityGross": "21173915535375",
        "liquidityNet": "-21173915535375",
        "tickIdx": "204130"
      },
      {
        "liquidityGross": "527279115156",
        "liquidityNet": "527279115156",
        "tickIdx": "204160"
      },
      {
        "liquidityGross": "81134504679928",
        "liquidityNet": "81134504679928",
        "tickIdx": "204170"
      },
      {
        "liquidityGross": "1004655231506",
        "liquidityNet": "1004655231506",
        "tickIdx": "204180"
      },
      {
        "liquidityGross": "697819740796349",
        "liquidityNet": "696765182566037",
        "tickIdx": "204200"
      },
      {
        "liquidityGross": "2962633523350",
        "liquidityNet": "-2962633523350",
        "tickIdx": "204220"
      },
      {
        "liquidityGross": "954979947124",
        "liquidityNet": "-954979947124",
        "tickIdx": "204240"
      },
      {
        "liquidityGross": "2298787003010616",
        "liquidityNet": "2298787003010616",
        "tickIdx": "204260"
      },
      {
        "liquidityGross": "1004655231506",
        "liquidityNet": "-1004655231506",
        "tickIdx": "204280"
      },
      {
        "liquidityGross": "2268108510133",
        "liquidityNet": "-2268108510133",
        "tickIdx": "204330"
      },
      {
        "liquidityGross": "1189417270755793",
        "liquidityNet": "-205167652606593",
        "tickIdx": "204340"
      },
      {
        "liquidityGross": "46922130405323",
        "liquidityNet": "46922130405323",
        "tickIdx": "204360"
      },
      {
        "liquidityGross": "519981051129559",
        "liquidityNet": "-497297810940347",
        "tickIdx": "204380"
      },
      {
        "liquidityGross": "86249643007776",
        "liquidityNet": "86249643007776",
        "tickIdx": "204390"
      },
      {
        "liquidityGross": "3990344019813",
        "liquidityNet": "-3990344019813",
        "tickIdx": "204420"
      },
      {
        "liquidityGross": "94050936873876",
        "liquidityNet": "-94050936873876",
        "tickIdx": "204470"
      },
      {
        "liquidityGross": "9024164118422",
        "liquidityNet": "9024164118422",
        "tickIdx": "204490"
      },
      {
        "liquidityGross": "10",
        "liquidityNet": "-10",
        "tickIdx": "204500"
      },
      {
        "liquidityGross": "21821905",
        "liquidityNet": "21821905",
        "tickIdx": "204530"
      },
      {
        "liquidityGross": "19358288239660",
        "liquidityNet": "-19358288239660",
        "tickIdx": "204540"
      },
      {
        "liquidityGross": "33615509923089",
        "liquidityNet": "-33615509923089",
        "tickIdx": "204550"
      },
      {
        "liquidityGross": "17618420282610",
        "liquidityNet": "17618420282610",
        "tickIdx": "204570"
      },
      {
        "liquidityGross": "23695521257064",
        "liquidityNet": "-23695521257064",
        "tickIdx": "204600"
      },
      {
        "liquidityGross": "1341183578617033",
        "liquidityNet": "1341183578617033",
        "tickIdx": "204610"
      },
      {
        "liquidityGross": "170673103538494",
        "liquidityNet": "37620554049366",
        "tickIdx": "204620"
      },
      {
        "liquidityGross": "39197183406877",
        "liquidityNet": "39197183406877",
        "tickIdx": "204630"
      },
      {
        "liquidityGross": "394387707481",
        "liquidityNet": "394387707481",
        "tickIdx": "204640"
      },
      {
        "liquidityGross": "84644554722165",
        "liquidityNet": "84644554722165",
        "tickIdx": "204650"
      },
      {
        "liquidityGross": "130330633417360",
        "liquidityNet": "-130330633417360",
        "tickIdx": "204660"
      },
      {
        "liquidityGross": "76428208044276",
        "liquidityNet": "-1966158769478",
        "tickIdx": "204670"
      },
      {
        "liquidityGross": "4495985696547137",
        "liquidityNet": "-4495985696547137",
        "tickIdx": "204700"
      },
      {
        "liquidityGross": "744623171956822",
        "liquidityNet": "-721318221347292",
        "tickIdx": "204710"
      },
      {
        "liquidityGross": "52991559159211",
        "liquidityNet": "-52991559159211",
        "tickIdx": "204720"
      },
      {
        "liquidityGross": "1290203203245374",
        "liquidityNet": "-1290203203245374",
        "tickIdx": "204740"
      },
      {
        "liquidityGross": "1087800237554443",
        "liquidityNet": "1087800237554443",
        "tickIdx": "204750"
      },
      {
        "liquidityGross": "58556667865517",
        "liquidityNet": "-58556667865517",
        "tickIdx": "204790"
      },
      {
        "liquidityGross": "8811768495241",
        "liquidityNet": "8811768495241",
        "tickIdx": "204800"
      },
      {
        "liquidityGross": "8811768495241",
        "liquidityNet": "-8811768495241",
        "tickIdx": "204820"
      },
      {
        "liquidityGross": "262121163150",
        "liquidityNet": "-262068004012",
        "tickIdx": "204860"
      },
      {
        "liquidityGross": "17372156034119",
        "liquidityNet": "-17372156034119",
        "tickIdx": "204880"
      },
      {
        "liquidityGross": "3777730624536",
        "liquidityNet": "-3777730624536",
        "tickIdx": "204890"
      },
      {
        "liquidityGross": "26579569",
        "liquidityNet": "-26579569",
        "tickIdx": "204900"
      },
      {
        "liquidityGross": "910271637044215",
        "liquidityNet": "-910271637044215",
        "tickIdx": "204910"
      },
      {
        "liquidityGross": "6510470788869",
        "liquidityNet": "-6510470788869",
        "tickIdx": "204930"
      },
      {
        "liquidityGross": "35112050975070",
        "liquidityNet": "-35112050975070",
        "tickIdx": "204950"
      },
      {
        "liquidityGross": "5708588153368",
        "liquidityNet": "5708588153368",
        "tickIdx": "204960"
      },
      {
        "liquidityGross": "37231024637399",
        "liquidityNet": "-37231024637399",
        "tickIdx": "204970"
      },
      {
        "liquidityGross": "939053049297",
        "liquidityNet": "-939053049297",
        "tickIdx": "204990"
      },
      {
        "liquidityGross": "15213000014794",
        "liquidityNet": "-15213000014794",
        "tickIdx": "205000"
      },
      {
        "liquidityGross": "114005757282180",
        "liquidityNet": "-109396489161074",
        "tickIdx": "205010"
      },
      {
        "liquidityGross": "3258972779416",
        "liquidityNet": "3258972779416",
        "tickIdx": "205070"
      },
      {
        "liquidityGross": "3258972779416",
        "liquidityNet": "-3258972779416",
        "tickIdx": "205090"
      },
      {
        "liquidityGross": "3815506156622",
        "liquidityNet": "3815506156622",
        "tickIdx": "205140"
      },
      {
        "liquidityGross": "72247538655926",
        "liquidityNet": "72247538655926",
        "tickIdx": "205180"
      },
      {
        "liquidityGross": "5708588153368",
        "liquidityNet": "-5708588153368",
        "tickIdx": "205190"
      },
      {
        "liquidityGross": "2154791887441158",
        "liquidityNet": "-2154791887441158",
        "tickIdx": "205200"
      },
      {
        "liquidityGross": "12887701135087",
        "liquidityNet": "12887701135087",
        "tickIdx": "205210"
      },
      {
        "liquidityGross": "2413943077381",
        "liquidityNet": "-2413943077381",
        "tickIdx": "205220"
      },
      {
        "liquidityGross": "12887701135087",
        "liquidityNet": "-12887701135087",
        "tickIdx": "205240"
      },
      {
        "liquidityGross": "2118391249583235",
        "liquidityNet": "2118391249583235",
        "tickIdx": "205250"
      },
      {
        "liquidityGross": "92409582075501",
        "liquidityNet": "92409582075501",
        "tickIdx": "205260"
      },
      {
        "liquidityGross": "2177818484237818",
        "liquidityNet": "-2177218599761408",
        "tickIdx": "205270"
      },
      {
        "liquidityGross": "108888109689097",
        "liquidityNet": "-75931054461905",
        "tickIdx": "205280"
      },
      {
        "liquidityGross": "41769168482150",
        "liquidityNet": "-41769168482150",
        "tickIdx": "205290"
      },
      {
        "liquidityGross": "163892614366023",
        "liquidityNet": "-163892614366023",
        "tickIdx": "205300"
      },
      {
        "liquidityGross": "35601822000",
        "liquidityNet": "35601822000",
        "tickIdx": "205330"
      },
      {
        "liquidityGross": "7721290634729",
        "liquidityNet": "-7721290634729",
        "tickIdx": "205340"
      },
      {
        "liquidityGross": "35413176029881",
        "liquidityNet": "35413176029881",
        "tickIdx": "205350"
      },
      {
        "liquidityGross": "35601822000",
        "liquidityNet": "-35601822000",
        "tickIdx": "205370"
      },
      {
        "liquidityGross": "58520881195404",
        "liquidityNet": "56032323483674",
        "tickIdx": "205380"
      },
      {
        "liquidityGross": "62750419706172",
        "liquidityNet": "-62750419706172",
        "tickIdx": "205390"
      },
      {
        "liquidityGross": "2535046243868695",
        "liquidityNet": "-2479164301850561",
        "tickIdx": "205420"
      },
      {
        "liquidityGross": "221591903313724",
        "liquidityNet": "221591903313724",
        "tickIdx": "205430"
      },
      {
        "liquidityGross": "2369908845622768",
        "liquidityNet": "-2337388845635494",
        "tickIdx": "205460"
      },
      {
        "liquidityGross": "167222976955551",
        "liquidityNet": "167222976955551",
        "tickIdx": "205490"
      },
      {
        "liquidityGross": "5141291429292",
        "liquidityNet": "-5141291429292",
        "tickIdx": "205500"
      },
      {
        "liquidityGross": "35547267802725",
        "liquidityNet": "-35547267802725",
        "tickIdx": "205510"
      },
      {
        "liquidityGross": "167222976955551",
        "liquidityNet": "-167222976955551",
        "tickIdx": "205530"
      },
      {
        "liquidityGross": "22741687028344",
        "liquidityNet": "22741687028344",
        "tickIdx": "205560"
      },
      {
        "liquidityGross": "55695768449144",
        "liquidityNet": "-10644388542016",
        "tickIdx": "205590"
      },
      {
        "liquidityGross": "22741687028344",
        "liquidityNet": "-22741687028344",
        "tickIdx": "205610"
      },
      {
        "liquidityGross": "22525689953564",
        "liquidityNet": "-22525689953564",
        "tickIdx": "205630"
      },
      {
        "liquidityGross": "46922130405323",
        "liquidityNet": "-46922130405323",
        "tickIdx": "205800"
      },
      {
        "liquidityGross": "484834570048165",
        "liquidityNet": "-484834570048165",
        "tickIdx": "205850"
      },
      {
        "liquidityGross": "1653347163134",
        "liquidityNet": "-1653347163134",
        "tickIdx": "205870"
      },
      {
        "liquidityGross": "5921513161873",
        "liquidityNet": "5921513161873",
        "tickIdx": "205920"
      },
      {
        "liquidityGross": "5921513161873",
        "liquidityNet": "-5921513161873",
        "tickIdx": "205930"
      },
      {
        "liquidityGross": "14566093142378",
        "liquidityNet": "-14566093142378",
        "tickIdx": "205990"
      },
      {
        "liquidityGross": "4132589508320855",
        "liquidityNet": "4132589508320855",
        "tickIdx": "206010"
      },
      {
        "liquidityGross": "821491371774",
        "liquidityNet": "-821491371774",
        "tickIdx": "206030"
      },
      {
        "liquidityGross": "36825342664461",
        "liquidityNet": "-36825342664461",
        "tickIdx": "206060"
      },
      {
        "liquidityGross": "36210029905465",
        "liquidityNet": "-36210029905465",
        "tickIdx": "206110"
      },
      {
        "liquidityGross": "382765298101521",
        "liquidityNet": "382765298101521",
        "tickIdx": "206130"
      },
      {
        "liquidityGross": "8070561896919252",
        "liquidityNet": "-8070561896919252",
        "tickIdx": "206190"
      },
      {
        "liquidityGross": "118797794071095",
        "liquidityNet": "-118797794071095",
        "tickIdx": "206220"
      },
      {
        "liquidityGross": "30410086366488",
        "liquidityNet": "-30410086366488",
        "tickIdx": "206250"
      },
      {
        "liquidityGross": "477197831652989",
        "liquidityNet": "-477197831652989",
        "tickIdx": "206260"
      },
      {
        "liquidityGross": "5643802559471404",
        "liquidityNet": "2993753263949656",
        "tickIdx": "206290"
      },
      {
        "liquidityGross": "744366934478",
        "liquidityNet": "-744366934478",
        "tickIdx": "206310"
      },
      {
        "liquidityGross": "4132589508320855",
        "liquidityNet": "-4132589508320855",
        "tickIdx": "206320"
      },
      {
        "liquidityGross": "122973486683145",
        "liquidityNet": "122973486683145",
        "tickIdx": "206370"
      },
      {
        "liquidityGross": "128273656740017",
        "liquidityNet": "-128273656740017",
        "tickIdx": "206380"
      },
      {
        "liquidityGross": "5517325292675",
        "liquidityNet": "5517325292675",
        "tickIdx": "206410"
      },
      {
        "liquidityGross": "5517325292675",
        "liquidityNet": "-5517325292675",
        "tickIdx": "206450"
      },
      {
        "liquidityGross": "216130743921190",
        "liquidityNet": "-216130743921190",
        "tickIdx": "206460"
      },
      {
        "liquidityGross": "46445625148087",
        "liquidityNet": "-46445625148087",
        "tickIdx": "206530"
      },
      {
        "liquidityGross": "10456351239484",
        "liquidityNet": "-10456351239484",
        "tickIdx": "206560"
      },
      {
        "liquidityGross": "39249186082639",
        "liquidityNet": "39249186082639",
        "tickIdx": "206690"
      },
      {
        "liquidityGross": "39249186082639",
        "liquidityNet": "-39249186082639",
        "tickIdx": "206710"
      },
      {
        "liquidityGross": "71560570730",
        "liquidityNet": "-71560570730",
        "tickIdx": "206850"
      },
      {
        "liquidityGross": "1060355204688",
        "liquidityNet": "-1060355204688",
        "tickIdx": "207040"
      },
      {
        "liquidityGross": "21599131813462",
        "liquidityNet": "-21599131813462",
        "tickIdx": "207090"
      },
      {
        "liquidityGross": "116526800732",
        "liquidityNet": "-116526800732",
        "tickIdx": "207190"
      },
      {
        "liquidityGross": "23690981880736",
        "liquidityNet": "23690981880736",
        "tickIdx": "207200"
      },
      {
        "liquidityGross": "29966490820790768",
        "liquidityNet": "-29966490820790768",
        "tickIdx": "207240"
      },
      {
        "liquidityGross": "108381237000",
        "liquidityNet": "-108381237000",
        "tickIdx": "207250"
      },
      {
        "liquidityGross": "37137476019528",
        "liquidityNet": "-37137476019528",
        "tickIdx": "207280"
      },
      {
        "liquidityGross": "1457334165987",
        "liquidityNet": "-1457334165987",
        "tickIdx": "207330"
      },
      {
        "liquidityGross": "18550684233560",
        "liquidityNet": "-18550684233560",
        "tickIdx": "207350"
      },
      {
        "liquidityGross": "1049687190863",
        "liquidityNet": "-1049687190863",
        "tickIdx": "207380"
      },
      {
        "liquidityGross": "6783329557647",
        "liquidityNet": "-6783329557647",
        "tickIdx": "207540"
      },
      {
        "liquidityGross": "13425333082288737",
        "liquidityNet": "-13425333082288737",
        "tickIdx": "207590"
      },
      {
        "liquidityGross": "17618420282610",
        "liquidityNet": "-17618420282610",
        "tickIdx": "207670"
      },
      {
        "liquidityGross": "1024516309504589",
        "liquidityNet": "-1024516309504589",
        "tickIdx": "207760"
      },
      {
        "liquidityGross": "33663120308853",
        "liquidityNet": "-33663120308853",
        "tickIdx": "207900"
      },
      {
        "liquidityGross": "8",
        "liquidityNet": "-8",
        "tickIdx": "207910"
      },
      {
        "liquidityGross": "227351883552117",
        "liquidityNet": "-227351883552117",
        "tickIdx": "208180"
      },
      {
        "liquidityGross": "96891010503",
        "liquidityNet": "-96891010503",
        "tickIdx": "208250"
      },
      {
        "liquidityGross": "3047476124369",
        "liquidityNet": "-3047476124369",
        "tickIdx": "208290"
      },
      {
        "liquidityGross": "469142103728099",
        "liquidityNet": "-469142103728099",
        "tickIdx": "208300"
      },
      {
        "liquidityGross": "15816971942115",
        "liquidityNet": "-15816971942115",
        "tickIdx": "208430"
      },
      {
        "liquidityGross": "54743213803487",
        "liquidityNet": "-54743213803487",
        "tickIdx": "208520"
      },
      {
        "liquidityGross": "1238910149582598",
        "liquidityNet": "-1238910149582598",
        "tickIdx": "208650"
      },
      {
        "liquidityGross": "1759243635560",
        "liquidityNet": "-1759243635560",
        "tickIdx": "208760"
      },
      {
        "liquidityGross": "116383157570658",
        "liquidityNet": "-116383157570658",
        "tickIdx": "208870"
      },
      {
        "liquidityGross": "14039449939",
        "liquidityNet": "-14039449939",
        "tickIdx": "209120"
      },
      {
        "liquidityGross": "34367459497789",
        "liquidityNet": "-34367459497789",
        "tickIdx": "209140"
      },
      {
        "liquidityGross": "4225060974401",
        "liquidityNet": "-4225060974401",
        "tickIdx": "209310"
      },
      {
        "liquidityGross": "9059350051728",
        "liquidityNet": "-9059350051728",
        "tickIdx": "209470"
      },
      {
        "liquidityGross": "1084560033827482",
        "liquidityNet": "-1084560033827482",
        "tickIdx": "209730"
      },
      {
        "liquidityGross": "191701986657523",
        "liquidityNet": "-191701986657523",
        "tickIdx": "209900"
      },
      {
        "liquidityGross": "7222293479401010",
        "liquidityNet": "-7222293479401010",
        "tickIdx": "210120"
      },
      {
        "liquidityGross": "228699723058089",
        "liquidityNet": "-228699723058089",
        "tickIdx": "210380"
      },
      {
        "liquidityGross": "69273863244383",
        "liquidityNet": "-69273863244383",
        "tickIdx": "210810"
      },
      {
        "liquidityGross": "13348687223838",
        "liquidityNet": "-13348687223838",
        "tickIdx": "210830"
      },
      {
        "liquidityGross": "2649113876628352",
        "liquidityNet": "-2649113876628352",
        "tickIdx": "211310"
      },
      {
        "liquidityGross": "2659554920775",
        "liquidityNet": "-2659554920775",
        "tickIdx": "211440"
      },
      {
        "liquidityGross": "4473176297160",
        "liquidityNet": "-4473176297160",
        "tickIdx": "211470"
      },
      {
        "liquidityGross": "1367464418001",
        "liquidityNet": "-1366052033351",
        "tickIdx": "211550"
      },
      {
        "liquidityGross": "706192325",
        "liquidityNet": "-706192325",
        "tickIdx": "211560"
      },
      {
        "liquidityGross": "4860235671811",
        "liquidityNet": "-4860235671811",
        "tickIdx": "212080"
      },
      {
        "liquidityGross": "4554637977519",
        "liquidityNet": "-4554637977519",
        "tickIdx": "212160"
      },
      {
        "liquidityGross": "1374517388397174",
        "liquidityNet": "-1374517388397174",
        "tickIdx": "212350"
      },
      {
        "liquidityGross": "187004715148085",
        "liquidityNet": "-187004715148085",
        "tickIdx": "212510"
      },
      {
        "liquidityGross": "13452407513465956",
        "liquidityNet": "-13452407513465956",
        "tickIdx": "212600"
      },
      {
        "liquidityGross": "504391575228",
        "liquidityNet": "-504391575228",
        "tickIdx": "213060"
      },
      {
        "liquidityGross": "2135569372348945",
        "liquidityNet": "-2135569372348945",
        "tickIdx": "214170"
      },
      {
        "liquidityGross": "419968624358816",
        "liquidityNet": "-419968624358816",
        "tickIdx": "214210"
      },
      {
        "liquidityGross": "102499926684047",
        "liquidityNet": "-102499926684047",
        "tickIdx": "216410"
      },
      {
        "liquidityGross": "470800979737",
        "liquidityNet": "-470800979737",
        "tickIdx": "217600"
      },
      {
        "liquidityGross": "123988887729631",
        "liquidityNet": "-123988887729631",
        "tickIdx": "218060"
      },
      {
        "liquidityGross": "224082011199",
        "liquidityNet": "-224082011199",
        "tickIdx": "219600"
      },
      {
        "liquidityGross": "498531057962448",
        "liquidityNet": "-498531057962448",
        "tickIdx": "221220"
      },
      {
        "liquidityGross": "1131670065466301",
        "liquidityNet": "-1131670065466301",
        "tickIdx": "221990"
      },
      {
        "liquidityGross": "63150",
        "liquidityNet": "-63150",
        "tickIdx": "222430"
      },
      {
        "liquidityGross": "43059560597867",
        "liquidityNet": "-43059560597867",
        "tickIdx": "223340"
      },
      {
        "liquidityGross": "1142415145764782",
        "liquidityNet": "-1142415145764782",
        "tickIdx": "223370"
      },
      {
        "liquidityGross": "35531670596",
        "liquidityNet": "-35531670596",
        "tickIdx": "225630"
      },
      {
        "liquidityGross": "34337229047",
        "liquidityNet": "-34337229047",
        "tickIdx": "225700"
      },
      {
        "liquidityGross": "21549469604675",
        "liquidityNet": "-21381469789743",
        "tickIdx": "230270"
      },
      {
        "liquidityGross": "1356881278244106196",
        "liquidityNet": "1356881278244106196",
        "tickIdx": "238030"
      },
      {
        "liquidityGross": "1356881278244106196",
        "liquidityNet": "-1356881278244106196",
        "tickIdx": "238040"
      },
      {
        "liquidityGross": "426144530607",
        "liquidityNet": "-426144530607",
        "tickIdx": "242650"
      },
      {
        "liquidityGross": "280102524730",
        "liquidityNet": "-280102524730",
        "tickIdx": "249240"
      },
      {
        "liquidityGross": "158140808402",
        "liquidityNet": "158140808402",
        "tickIdx": "260820"
      },
      {
        "liquidityGross": "24250664520",
        "liquidityNet": "-24250664520",
        "tickIdx": "269390"
      },
      {
        "liquidityGross": "2998064053514881",
        "liquidityNet": "2998029234933125",
        "tickIdx": "276300"
      },
      {
        "liquidityGross": "3008582281165689",
        "liquidityNet": "-2987584474068099",
        "tickIdx": "276320"
      },
      {
        "liquidityGross": "2266789968",
        "liquidityNet": "-2266789968",
        "tickIdx": "292530"
      },
      {
        "liquidityGross": "2273693713",
        "liquidityNet": "-2273693713",
        "tickIdx": "292560"
      },
      {
        "liquidityGross": "44739669244",
        "liquidityNet": "-44739669244",
        "tickIdx": "292600"
      },
      {
        "liquidityGross": "28436567813",
        "liquidityNet": "-28436567813",
        "tickIdx": "310340"
      },
      {
        "liquidityGross": "3216578076775223",
        "liquidityNet": "3216578076775223",
        "tickIdx": "344450"
      },
      {
        "liquidityGross": "3216578076775223",
        "liquidityNet": "-3216578076775223",
        "tickIdx": "345410"
      },
      {
        "liquidityGross": "1398391284452690",
        "liquidityNet": "1398391284452690",
        "tickIdx": "349460"
      },
      {
        "liquidityGross": "2525400803780",
        "liquidityNet": "-2525400803780",
        "tickIdx": "349620"
      },
      {
        "liquidityGross": "127004648638536",
        "liquidityNet": "127004648638536",
        "tickIdx": "351240"
      },
      {
        "liquidityGross": "166765074643655",
        "liquidityNet": "-122036674458985",
        "tickIdx": "352340"
      },
      {
        "liquidityGross": "1252272593613901",
        "liquidityNet": "-1252272593613901",
        "tickIdx": "352820"
      },
      {
        "liquidityGross": "47846832113457898",
        "liquidityNet": "47843396480882960",
        "tickIdx": "354570"
      },
      {
        "liquidityGross": "1021617313228699",
        "liquidityNet": "1021617313228699",
        "tickIdx": "354900"
      },
      {
        "liquidityGross": "13379584971254501",
        "liquidityNet": "13379584971254501",
        "tickIdx": "356050"
      },
      {
        "liquidityGross": "511481352845",
        "liquidityNet": "511481352845",
        "tickIdx": "356900"
      },
      {
        "liquidityGross": "13379584971254501",
        "liquidityNet": "-13379584971254501",
        "tickIdx": "357040"
      },
      {
        "liquidityGross": "3371930637656804",
        "liquidityNet": "3371930637656804",
        "tickIdx": "357500"
      },
      {
        "liquidityGross": "1017111148",
        "liquidityNet": "-1017111148",
        "tickIdx": "357790"
      },
      {
        "liquidityGross": "297698757815726047",
        "liquidityNet": "297698757815726047",
        "tickIdx": "358040"
      },
      {
        "liquidityGross": "297698757815726047",
        "liquidityNet": "-297698757815726047",
        "tickIdx": "358080"
      },
      {
        "liquidityGross": "3371930637656804",
        "liquidityNet": "-3371930637656804",
        "tickIdx": "359140"
      },
      {
        "liquidityGross": "47845114297170429",
        "liquidityNet": "-47845114297170429",
        "tickIdx": "359260"
      },
      {
        "liquidityGross": "1021617313228699",
        "liquidityNet": "-1021617313228699",
        "tickIdx": "359660"
      },
      {
        "liquidityGross": "103724168037440",
        "liquidityNet": "-103724168037440",
        "tickIdx": "361500"
      },
      {
        "liquidityGross": "11099171960",
        "liquidityNet": "11099171960",
        "tickIdx": "391460"
      },
      {
        "liquidityGross": "1205661623580111",
        "liquidityNet": "1205661623580111",
        "tickIdx": "407550"
      },
      {
        "liquidityGross": "1350483406084573",
        "liquidityNet": "-1350483406084573",
        "tickIdx": "414490"
      },
      {
        "liquidityGross": "398290794261",
        "liquidityNet": "-398290794261",
        "tickIdx": "759890"
      },
      {
        "liquidityGross": "45039377467845144",
        "liquidityNet": "-45039377467845144",
        "tickIdx": "887270"
      }
    ]`

// createRealisticV3Pool creates a test fixture based on a snapshot of the
// USDC/WETH 0.3% pool on Ethereum Mainnet, using a comprehensive list of ticks.
func createRealisticV3Pool(t *testing.T) uniswapv3.PoolView {
	pool := uniswapv3.PoolView{
		PoolViewMinimal: uniswapv3.PoolViewMinimal{
			ID:           1,
			Token0:       0, // USDC (6 decimals)
			Token1:       1, // WETH (18 decimals)
			Fee:          3000,
			TickSpacing:  10, // Common for stable/volatile pairs
			Tick:         193540,
			Liquidity:    fromString("4411461329627947710"),                // Liquidity at the starting tick
			SqrtPriceX96: fromString("1262831046415630070062062910819682"), // Price at tick 0
		},
		Ticks: make([]ticks.TickInfo, 0, len(rawTicks)),
	}

	err := json.Unmarshal([]byte(rawTicksJson), &rawTicks)
	if err != nil {
		panic(err)
	}
	for _, rt := range rawTicks {
		tickIdx, _ := new(big.Int).SetString(rt.TickIdx, 10)
		t := ticks.TickInfo{
			Index:        tickIdx.Int64(),
			LiquidityNet: fromString(rt.LiquidityNet),
		}

		if t.LiquidityNet.Sign() != 0 {
			pool.Ticks = append(pool.Ticks, t)
		}

	}

	return pool
}
func TestSimulateSwap_ExactInput_WithRealisticPool(t *testing.T) {
	pool := createRealisticV3Pool(t)
	// The calculator is now stateless, so we can call the public functions directly.

	testCases := []struct {
		description string
		zeroForOne  bool
		amountIn    *big.Int
		expectedOut *big.Int
	}{
		// --- Swaps: USDC (Token0) for WETH (Token1) ---
		{
			description: "Swap small amount: 1,000 USDC for WETH",
			zeroForOne:  true,
			amountIn:    big.NewInt(1_000e6), // 1,000 USDC (6 decimals)
			expectedOut: big.NewInt(253294014434655388),
		},
		{
			description: "Swap medium amount: 100,000 USDC for WETH (likely crosses a tick)",
			zeroForOne:  true,
			amountIn:    big.NewInt(100_000e6), // 100,000 USDC
			expectedOut: fromString("25320371561927115634"),
		},
		{
			description: "Swap large amount: 1,000,000 USDC for WETH (high price impact)",
			zeroForOne:  true,
			amountIn:    big.NewInt(1_000_000e6), // 1,000,000 USDC
			expectedOut: fromString("252382792995323662042"),
		},
		{
			description: "Swap tiny amount: 1 USDC for WETH",
			zeroForOne:  true,
			amountIn:    big.NewInt(1e6), // 1 USDC
			expectedOut: big.NewInt(253294925960028),
		},

		// --- Swaps: WETH (Token1) for USDC (Token0) ---
		{
			description: "Swap small amount: 0.1 WETH for USDC",
			zeroForOne:  false,
			amountIn:    fromString("100000000000000000"), // 0.1 WETH (18 decimals)
			expectedOut: big.NewInt(392430911),
		},
		{
			description: "Swap medium amount: 10 WETH for USDC (likely crosses a tick)",
			zeroForOne:  false,
			amountIn:    fromString("10000000000000000000"), // 10 WETH
			expectedOut: big.NewInt(39237583289),
		},
		{
			description: "Swap large amount: 100 WETH for USDC (high price impact)",
			zeroForOne:  false,
			amountIn:    fromString("100000000000000000000"), // 100 WETH
			expectedOut: big.NewInt(391878407478),
		},
		{
			description: "Swap tiny amount: 0.0001 WETH for USDC",
			zeroForOne:  false,
			amountIn:    fromString("100000000000000"), // 0.0001 WETH
			expectedOut: big.NewInt(392431),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var tokenInID uint64
			if tc.zeroForOne {
				tokenInID = 0
			} else {
				tokenInID = 1
			}

			amountOut, newPoolState, err := SimulateExactInSwap(tc.amountIn, nil, tokenInID, pool)
			require.NoError(t, err)

			// Assert that the calculated amount out matches the known-good value.
			assert.Equal(t, tc.expectedOut.String(), amountOut.String())

			// Basic sanity check on the new pool state
			assert.NotEqual(t, pool.SqrtPriceX96.String(), newPoolState.SqrtPriceX96.String(), "SqrtPriceX96 should change after a swap")

		})
	}

}

func TestSimulateSwap_ExactOutput_WithRealisticPool(t *testing.T) {
	pool := createRealisticV3Pool(t)

	testCases := []struct {
		description string
		zeroForOne  bool
		amountOut   *big.Int
		expectedIn  *big.Int
	}{
		// --- Swaps to get a specific amount of WETH (Token1) by paying with USDC (Token0) ---
		{
			description: "Get ~0.25 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(big.NewInt(253294014434655388)),
			expectedIn:  big.NewInt(1000000000),
		},
		{
			description: "Get ~25.32 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(fromString("25320371561927115634")),
			expectedIn:  big.NewInt(100000000000),
		},
		{
			description: "Get ~252.38 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(fromString("252382792995323662042")),
			expectedIn:  big.NewInt(1000000000000),
		},
		{
			description: "Get tiny amount of WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(big.NewInt(253294925960028)),
			expectedIn:  big.NewInt(1000000),
		},

		// --- Swaps to get a specific amount of USDC (Token0) by paying with WETH (Token1) ---
		{
			description: "Get ~392.43 USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(392430911)),
			expectedIn:  fromString("99999999844101905"),
		},
		{
			description: "Get ~39,237.58 USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(39237583289)),
			expectedIn:  fromString("9999999999972204976"),
		},
		{
			description: "Get ~391,878.40 USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(391878407478)),
			expectedIn:  fromString("99999999999956338180"),
		},
		{
			description: "Get tiny amount of USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(392431)),
			expectedIn:  fromString("99999880874753"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var tokenInID uint64
			if tc.zeroForOne {
				// We are paying with Token0 (USDC) to get Token1 (WETH)
				tokenInID = 0
			} else {
				// We are paying with Token1 (WETH) to get Token0 (USDC)
				tokenInID = 1
			}

			amountIn, newPoolState, err := SimulateExactOutSwap(tc.amountOut, nil, tokenInID, pool)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedIn.String(), amountIn.String())

			// Basic sanity check on the new pool state
			assert.NotEqual(t, pool.SqrtPriceX96.String(), newPoolState.SqrtPriceX96.String(), "SqrtPriceX96 should change after a swap")
		})
	}
}

// TestGetAmountOut verifies the simplified GetAmountOut function.
func TestGetAmountOut(t *testing.T) {
	pool := createRealisticV3Pool(t)

	testCases := []struct {
		description string
		zeroForOne  bool
		amountIn    *big.Int
		expectedOut *big.Int
	}{
		// --- Swaps: USDC (Token0) for WETH (Token1) ---
		{
			description: "Swap small amount: 1,000 USDC for WETH",
			zeroForOne:  true,
			amountIn:    big.NewInt(1_000e6), // 1,000 USDC (6 decimals)
			expectedOut: big.NewInt(253294014434655388),
		},
		{
			description: "Swap medium amount: 100,000 USDC for WETH (likely crosses a tick)",
			zeroForOne:  true,
			amountIn:    big.NewInt(100_000e6), // 100,000 USDC
			expectedOut: fromString("25320371561927115634"),
		},
		{
			description: "Swap large amount: 1,000,000 USDC for WETH (high price impact)",
			zeroForOne:  true,
			amountIn:    big.NewInt(1_000_000e6), // 1,000,000 USDC
			expectedOut: fromString("252382792995323662042"),
		},
		{
			description: "Swap tiny amount: 1 USDC for WETH",
			zeroForOne:  true,
			amountIn:    big.NewInt(1e6), // 1 USDC
			expectedOut: big.NewInt(253294925960028),
		},

		// --- Swaps: WETH (Token1) for USDC (Token0) ---
		{
			description: "Swap small amount: 0.1 WETH for USDC",
			zeroForOne:  false,
			amountIn:    fromString("100000000000000000"), // 0.1 WETH (18 decimals)
			expectedOut: big.NewInt(392430911),
		},
		{
			description: "Swap medium amount: 10 WETH for USDC (likely crosses a tick)",
			zeroForOne:  false,
			amountIn:    fromString("10000000000000000000"), // 10 WETH
			expectedOut: big.NewInt(39237583289),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var tokenInID uint64
			if tc.zeroForOne {
				tokenInID = 0
			} else {
				tokenInID = 1
			}

			// Call the function under test
			amountOut, err := GetAmountOut(tc.amountIn, nil, tokenInID, pool)
			require.NoError(t, err)

			// Assert that the calculated amount out matches the known-good value.
			assert.Equal(t, tc.expectedOut.String(), amountOut.String())
		})
	}
}

// TestGetAmountIn verifies the simplified GetAmountIn function.
func TestGetAmountIn(t *testing.T) {
	pool := createRealisticV3Pool(t)

	testCases := []struct {
		description string
		zeroForOne  bool
		amountOut   *big.Int
		expectedIn  *big.Int
	}{
		// --- Swaps to get a specific amount of WETH (Token1) by paying with USDC (Token0) ---
		{
			description: "Get ~0.25 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(big.NewInt(253294014434655388)),
			expectedIn:  big.NewInt(1000000000),
		},
		{
			description: "Get ~25.32 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(fromString("25320371561927115634")),
			expectedIn:  big.NewInt(100000000000),
		},
		{
			description: "Get ~252.38 WETH by paying with USDC",
			zeroForOne:  true,
			amountOut:   negBigInt(fromString("252382792995323662042")),
			expectedIn:  big.NewInt(1000000000000),
		},
		// --- Swaps to get a specific amount of USDC (Token0) by paying with WETH (Token1) ---
		{
			description: "Get ~392.43 USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(392430911)),
			expectedIn:  fromString("99999999844101905"),
		},
		{
			description: "Get ~39,237.58 USDC by paying with WETH",
			zeroForOne:  false,
			amountOut:   negBigInt(big.NewInt(39237583289)),
			expectedIn:  fromString("9999999999972204976"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			var tokenInID uint64
			if tc.zeroForOne {
				// We are paying with Token0 (USDC) to get Token1 (WETH)
				tokenInID = 0
			} else {
				// We are paying with Token1 (WETH) to get Token0 (USDC)
				tokenInID = 1
			}

			// Call the function under test
			amountIn, err := GetAmountIn(tc.amountOut, nil, tokenInID, pool)
			require.NoError(t, err)

			// Assert that the calculated amount in matches the known-good value.
			assert.Equal(t, tc.expectedIn.String(), amountIn.String())
		})
	}
}

// TestSimulateSwap_IdempotencyAndStateIsolation verifies that the simulation
// function does not mutate its inputs (idempotency) and that the returned
// new state is a proper partial deep copy, preventing side effects.
func TestSimulateSwap_IdempotencyAndStateIsolation(t *testing.T) {
	// 1. Arrange: Create the initial, pristine pool state.
	originalPool := createRealisticV3Pool(t)
	amountIn := big.NewInt(100_000e6) // 100,000 USDC
	tokenInID := uint64(0)

	// 2. Act: Run the simulation twice on the *same original state*.
	amountOut1, newPoolState1, err1 := SimulateExactInSwap(amountIn, nil, tokenInID, originalPool)
	require.NoError(t, err1, "First simulation should succeed")

	amountOut2, newPoolState2, err2 := SimulateExactInSwap(amountIn, nil, tokenInID, originalPool)
	require.NoError(t, err2, "Second simulation should succeed")

	// 3. Assert: Verify idempotency and state isolation.

	t.Run("Idempotency Check", func(t *testing.T) {
		// This proves that the first simulation did not mutate the 'originalPool' object.
		// If it had, the second simulation would have started from a different state
		// and produced a different result.
		assert.Equal(t, amountOut1.String(), amountOut2.String(), "Amount out should be identical on consecutive runs")
		assert.True(t, reflect.DeepEqual(newPoolState1, newPoolState2), "The new pool state should be identical on consecutive runs")
	})

	t.Run("Deep Copy Check (Mutable Fields)", func(t *testing.T) {
		// This proves that the mutable *big.Int fields in the new state are new
		// instances in memory, not just copies of the original pointers.
		assert.NotSame(t, originalPool.Liquidity, newPoolState1.Liquidity, "New state's Liquidity should be a new big.Int instance")
		assert.NotSame(t, originalPool.SqrtPriceX96, newPoolState1.SqrtPriceX96, "New state's SqrtPriceX96 should be a new big.Int instance")
	})

	t.Run("Shallow Copy Check (Immutable Fields)", func(t *testing.T) {
		// This proves the intentional performance optimization: the Ticks slice, which
		// is treated as read-only, shares its underlying memory pointer with the original.
		// We check this by comparing the memory address of the first element.
		require.True(t, len(originalPool.Ticks) > 0, "Pool must have ticks to perform this check")
		assert.Same(t, &originalPool.Ticks[0], &newPoolState1.Ticks[0], "Ticks slice should be a shallow copy, sharing the pointer")
	})

	t.Run("Result Isolation Check", func(t *testing.T) {
		// This is the definitive test. We modify the result of the first simulation
		// and verify that the result of the second simulation is not affected.
		// This proves that the two returned states are truly independent of each other.
		originalLiquidity2 := new(big.Int).Set(newPoolState2.Liquidity)

		// Mutate the result of the first simulation
		newPoolState1.Liquidity.Add(newPoolState1.Liquidity, big.NewInt(12345))

		// Assert that the second result remains unchanged
		assert.NotEqual(t, newPoolState1.Liquidity.String(), newPoolState2.Liquidity.String(), "Modifying state 1 should not affect state 2")
		assert.Equal(t, originalLiquidity2.String(), newPoolState2.Liquidity.String(), "State 2's liquidity should remain pristine")
	})
}

// TestGetSpotPrice provides a suite of tests for the GetSpotPrice function.
func TestGetSpotPrice(t *testing.T) {
	// This sqrtPriceX96 corresponds to a WETH/USDT price of ~3045.12 USDT per WETH
	sqrtPriceX96, _ := new(big.Int).SetString("4602761997227095498465462", 10)

	// Mock Pool: Assume Token 0 is WETH (18 decimals) and Token 1 is USDT (6 decimals)
	mockPool := uniswapv3.PoolView{
		PoolViewMinimal: uniswapv3.PoolViewMinimal{
			Token0:       0, // WETH
			Token1:       1, // USDT
			SqrtPriceX96: sqrtPriceX96,
		},
	}

	// Define test cases
	testCases := []struct {
		name          string
		tokenInID     uint64
		tokenOutID    uint64
		decimalsIn    uint8
		decimalsOut   uint8
		pool          uniswapv3.PoolView
		expectedPrice string
		expectError   bool
	}{
		{
			name:          "Native Direction: WETH (18) -> USDT (6)",
			tokenInID:     0,
			tokenOutID:    1,
			decimalsIn:    18,
			decimalsOut:   6,
			pool:          mockPool,
			expectedPrice: "3375031805",
			expectError:   false,
		},
		{
			name:          "Inverse Direction: USDT (6) -> WETH (18)",
			tokenInID:     1,
			tokenOutID:    0,
			decimalsIn:    6,
			decimalsOut:   18,
			pool:          mockPool,
			expectedPrice: "296293504072605",
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function under test
			spotPrice, err := GetSpotPrice(tc.tokenInID, tc.tokenOutID, tc.decimalsIn, tc.decimalsOut, tc.pool)

			// Check for expected error
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				return // End test here if error was expected
			}

			// Check if an unexpected error occurred
			if err != nil {
				t.Fatalf("Expected no error, but got: %v", err)
			}

			// Convert expected string to big.Int for comparison
			expectedBigInt, ok := new(big.Int).SetString(tc.expectedPrice, 10)
			if !ok {
				t.Fatalf("Invalid expectedPrice string in test case: %s", tc.expectedPrice)
			}

			// Compare the actual result with the expected result
			if spotPrice.Cmp(expectedBigInt) != 0 {
				t.Errorf("Mismatch in spot price.\nExpected: %s\nGot:      %s", expectedBigInt.String(), spotPrice.String())
			}

		})
	}
}
