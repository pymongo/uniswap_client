// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

interface IUniswapV2Router {
    function swapTokensForExactETH(
        uint amountOut, 
        uint amountInMax, 
        address[] calldata path, 
        address to, 
        uint deadline
    ) external returns (uint[] memory amounts);
}

contract SwapHelper {
    IUniswapV2Router public uniswapRouter;

    event SwapResult(string message, uint[] amounts);

    constructor() {
        uniswapRouter = IUniswapV2Router(0x5023882f4D1EC10544FCB2066abE9C1645E95AA0);
    }

    function swapTokensForExactETH(
        uint amountOut,
        uint amountInMax,
        address[] calldata path,
        address to,
        uint deadline
    ) external returns (uint[] memory amounts) {
        try uniswapRouter.swapTokensForExactETH(amountOut, amountInMax, path, to, deadline) returns (uint[] memory result) {
            emit SwapResult("Swap succeeded", result);
            return result;
        } catch {
            emit SwapResult("Swap failed", new uint[](0));
            revert("Swap execution reverted");
        }
    }
}