-- @tweak name="Nanite rewards" group="Currency"
-- @desc Multiplies Nanites rewards only, on top of whatever the units and
-- @desc nanites tweak already did. Units are never touched here.
-- @param NANITE_MULT label="Extra nanite multiplier" min=1 max=100 step=1 default=10
-- @param NANITES_CAP label="Largest nanites reward" min=0 max=10000000 step=10000 default=250000
-- Hugely buff NANITE rewards. Deterministic: multiplies AmountMin/AmountMax only
-- in GcRewardMoney blocks whose Currency=Nanites. Stacks on MoneyAndNanites5x
-- (x5 all money) -> nanites ~x50 total; units stay x5, never touched here.
NANITE_MULT = 10
NANITES_CAP = 250000  -- ceiling on the final nanites amount, after MoneyAndNanites5x has
                      -- already multiplied it. 0 = no ceiling.

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "NaniteRewardBuff.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\REWARDTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        { ["CURRENCY_MULT"] = { ["CURRENCY"]="Nanites", ["MULT"]=NANITE_MULT }, ["CAP"] = NANITES_CAP }
                    }
                }
            }
        }
    }
}
