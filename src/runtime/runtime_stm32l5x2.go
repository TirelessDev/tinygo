// +build stm32,stm32l5x2

package runtime

import (
	"machine"

	"github.com/sago35/device/stm32"
)

/*
   clock settings
   +-------------+-----------+
   | LSE         | 32.768khz |
   | SYSCLK      | 110mhz    |
   | HCLK        | 110mhz    |
   | APB1(PCLK1) | 110mhz    |
   | APB2(PCLK2) | 110mhz    |
   +-------------+-----------+
*/
const (
	HSE_STARTUP_TIMEOUT = 0x0500
	PLL_M               = 1
	PLL_N               = 55
	PLL_P               = 7 // RCC_PLLP_DIV7
	PLL_Q               = 2 // RCC_PLLQ_DIV2
	PLL_R               = 2 // RCC_PLLR_DIV2
)

/*
   timer settings used for tick and sleep.

   note: TICK_TIMER_FREQ and SLEEP_TIMER_FREQ are controlled by PLL / clock
   settings above, so must be kept in sync if the clock settings are changed.
*/
const (
	TICK_RATE        = 1000 // 1 KHz
	SLEEP_TIMER_IRQ  = stm32.IRQ_TIM15
	SLEEP_TIMER_FREQ = 110000000 // 110 MHz
	TICK_TIMER_IRQ   = stm32.IRQ_TIM16
	TICK_TIMER_FREQ  = 110000000 // 110 MHz
)

type arrtype = uint32

const asyncScheduler = false

func init() {
	initCLK()

	initSleepTimer(&timerInfo{
		EnableRegister: &stm32.RCC.APB2ENR,
		EnableFlag:     stm32.RCC_APB2ENR_TIM15EN,
		Device:         stm32.TIM15,
	})

	machine.UART0.Configure(machine.UARTConfig{})

	initTickTimer(&timerInfo{
		EnableRegister: &stm32.RCC.APB2ENR,
		EnableFlag:     stm32.RCC_APB2ENR_TIM16EN,
		Device:         stm32.TIM16,
	})
}

func putchar(c byte) {
	machine.UART0.WriteByte(c)
}

func initCLK() {

	// PWR_CLK_ENABLE
	stm32.RCC.APB1ENR1.SetBits(stm32.RCC_APB1ENR1_PWREN)
	_ = stm32.RCC.APB1ENR1.Get()

	// PWR_VOLTAGESCALING_CONFIG
	stm32.PWR.CR1.ReplaceBits(0, stm32.PWR_CR1_VOS_Msk, 0)
	_ = stm32.PWR.CR1.Get()

	// Initialize the High-Speed External Oscillator
	initOsc()

	// Set flash wait states (min 5 latency units) based on clock
	if (stm32.FLASH.ACR.Get() & 0xF) < 5 {
		stm32.FLASH.ACR.ReplaceBits(5, 0xF, 0)
	}

	// Ensure HCLK does not exceed max during transition
	stm32.RCC.CFGR.ReplaceBits(8<<stm32.RCC_CFGR_HPRE_Pos, stm32.RCC_CFGR_HPRE_Msk, 0)

	// Set SYSCLK source and wait
	// (3 = RCC_SYSCLKSOURCE_PLLCLK, 2=RCC_CFGR_SWS_Pos)
	stm32.RCC.CFGR.ReplaceBits(3, stm32.RCC_CFGR_SW_Msk, 0)
	for stm32.RCC.CFGR.Get()&(3<<2) != (3 << 2) {
	}

	// Set HCLK
	// (0 = RCC_SYSCLKSOURCE_PLLCLK)
	stm32.RCC.CFGR.ReplaceBits(0, stm32.RCC_CFGR_HPRE_Msk, 0)

	// Set flash wait states (max 5 latency units) based on clock
	if (stm32.FLASH.ACR.Get() & 0xF) > 5 {
		stm32.FLASH.ACR.ReplaceBits(5, 0xF, 0)
	}

	// Set APB1 and APB2 clocks (0 = DIV1)
	stm32.RCC.CFGR.ReplaceBits(0, stm32.RCC_CFGR_PPRE1_Msk, 0)
	stm32.RCC.CFGR.ReplaceBits(0, stm32.RCC_CFGR_PPRE2_Msk, 0)
}

func initOsc() {
	// Enable HSI, wait until ready
	stm32.RCC.CR.SetBits(stm32.RCC_CR_HSION)
	for !stm32.RCC.CR.HasBits(stm32.RCC_CR_HSIRDY) {
	}

	// Disable Backup domain protection
	if !stm32.PWR.CR1.HasBits(stm32.PWR_CR1_DBP) {
		stm32.PWR.CR1.SetBits(stm32.PWR_CR1_DBP)
		for !stm32.PWR.CR1.HasBits(stm32.PWR_CR1_DBP) {
		}
	}

	// Set LSE Drive to LOW
	stm32.RCC.BDCR.ReplaceBits(0, stm32.RCC_BDCR_LSEDRV_Msk, 0)

	// Enable LSE, wait until ready
	stm32.RCC.BDCR.SetBits(stm32.RCC_BDCR_LSEON)
	for !stm32.RCC.BDCR.HasBits(stm32.RCC_BDCR_LSEON) {
	}

	// Ensure LSESYS disabled
	stm32.RCC.BDCR.ClearBits(stm32.RCC_BDCR_LSESYSEN)
	for stm32.RCC.BDCR.HasBits(stm32.RCC_BDCR_LSESYSEN) {
	}

	// Enable HSI48, wait until ready
	stm32.RCC.CRRCR.SetBits(stm32.RCC_CRRCR_HSI48ON)
	for !stm32.RCC.CRRCR.HasBits(stm32.RCC_CRRCR_HSI48ON) {
	}

	// Disable the PLL, wait until disabled
	stm32.RCC.CR.ClearBits(stm32.RCC_CR_PLLON)
	for stm32.RCC.CR.HasBits(stm32.RCC_CR_PLLRDY) {
	}

	// Configure the PLL
	stm32.RCC.PLLCFGR.ReplaceBits(
		(1)| // 1 = RCC_PLLSOURCE_MSI
			(PLL_M-1)<<stm32.RCC_PLLCFGR_PLLM_Pos|
			(PLL_N<<stm32.RCC_PLLCFGR_PLLN_Pos)|
			(((PLL_Q>>1)-1)<<stm32.RCC_PLLCFGR_PLLQ_Pos)|
			(((PLL_R>>1)-1)<<stm32.RCC_PLLCFGR_PLLR_Pos)|
			(PLL_P<<stm32.RCC_PLLCFGR_PLLPDIV_Pos),
		stm32.RCC_PLLCFGR_PLLSRC_Msk|stm32.RCC_PLLCFGR_PLLM_Msk|
			stm32.RCC_PLLCFGR_PLLN_Msk|stm32.RCC_PLLCFGR_PLLP_Msk|
			stm32.RCC_PLLCFGR_PLLR_Msk|stm32.RCC_PLLCFGR_PLLPDIV_Msk,
		0)

	// Enable the PLL and PLL System Clock Output, wait until ready
	stm32.RCC.CR.SetBits(stm32.RCC_CR_PLLON)
	stm32.RCC.PLLCFGR.SetBits(stm32.RCC_PLLCFGR_PLLREN) // = RCC_PLL_SYSCLK
	for !stm32.RCC.CR.HasBits(stm32.RCC_CR_PLLRDY) {
	}

}
