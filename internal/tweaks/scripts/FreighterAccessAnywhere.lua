-- @tweak name="Freighter access anywhere" group="Ships"
-- @desc The freighter behaves as if the Matter Beam were installed, without
-- @desc building it or giving up a slot: its hyperdrive, which every freighter
-- @desc has, also carries the Matter Beam's Freighter_Teleport stat.
-- The Matter Beam (F_TELEPORT) does nothing but grant Freighter_Teleport 100;
-- the executable reads the stat. Adding the same bonus to the core hyperdrive
-- (F_HYPERDRIVE) gives every freighter the stat whether or not the beam is
-- installed. The bonus goes in after the hyperdrive's last stat bonus
-- (Freighter_Fleet_Boost), as the next GcStatsBonus in its list.

NMS_MOD_DEFINITION_CONTAINER =
{
["MOD_FILENAME"] = "FreighterAccessAnywhere.pak",
["MOD_AUTHOR"]   = "nmsbonker",
["MODIFICATIONS"] =
    {
        {
            ["MBIN_CHANGE_TABLE"] =
            {
                {
                    ["MBIN_FILE_SOURCE"] = "METADATA\REALITY\TABLES\NMS_REALITY_GCTECHNOLOGYTABLE.MBIN",
                    ["EXML_CHANGE_TABLE"] =
                    {
                        {
                            ["SPECIAL_KEY_WORDS"] = {"F_HYPERDRIVE", "Freighter_Fleet_Boost"},
                            ["SECTION_UP"]        = 2,
                            ["ADD"] = [[
				<Property name="StatBonuses" value="GcStatsBonus" _index="4">
					<Property name="Stat" value="GcStatsTypes">
						<Property name="StatsType" value="Freighter_Teleport" />
					</Property>
					<Property name="Bonus" value="100.000000" />
					<Property name="Level" value="1" />
				</Property>]]
                        }
                    }
                }
            }
        }
    }
}
