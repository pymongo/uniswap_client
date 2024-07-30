// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "forge-std/Script.sol";
import "../src/SwapHelper.sol";

contract DeploySwapHelper is Script {
    function run() external {
        vm.startBroadcast();
        
        SwapHelper swapHelper = new SwapHelper();
        
        vm.stopBroadcast();

        emit Deployed(address(swapHelper));
    }

    event Deployed(address indexed contractAddress);
}