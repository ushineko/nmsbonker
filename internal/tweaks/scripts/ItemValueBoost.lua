-- @tweak name="Item value" group="Economy"
-- @desc Multiplies the base value of every substance and product, which is what
-- @desc the galactic market prices are derived from. Buy prices rise with sell
-- @desc prices; the game computes both from the same number.
-- @param VALUE_MULT label="Item value multiplier" min=1 max=50 step=1 default=3 kind=float
-- Boost the market value of every item (substances + products) by multiplying
-- BaseValue x3 in the substance and product tables. BaseValue drives the galactic
-- market price, so this raises SELL value ~3x (and also buy price ~3x, since NMS
-- derives both from the same base).
VALUE_MULT = 3

local function triple(file)
  return {
    ["MBIN_FILE_SOURCE"] = file,
    ["EXML_CHANGE_TABLE"] = {
      { ["PRECEDING_KEY_WORDS"]="", ["MATH_OPERATION"]="*", ["REPLACE_TYPE"]="ALL",
        ["VALUE_CHANGE_TABLE"]={ {"BaseValue", VALUE_MULT} } }
    }
  }
end

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "ItemValueBoost.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        { ["MBIN_CHANGE_TABLE"] = {
            triple("METADATA\REALITY\TABLES\NMS_REALITY_GCPRODUCTTABLE.MBIN"),
            triple("METADATA\REALITY\TABLES\NMS_REALITY_GCSUBSTANCETABLE.MBIN")
        } }
    }
}
