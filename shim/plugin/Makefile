# Builds udsrelay-shim.3gx, a CTRPluginFramework plugin for the Luma3DS plugin loader.
# Adapted from CTRPluginFramework's TestPlugin Makefile; release build only.
#
# Needs devkitARM, libctru, libctrpf and 3gxtool (see shim/RUNBOOK.md). Build with build.sh:
# devkitPro's make rules break if the path contains a space, and this repository's does.
#---------------------------------------------------------------------------------
.SUFFIXES:
#---------------------------------------------------------------------------------

ifeq ($(strip $(DEVKITARM)),)
$(error "Please set DEVKITARM in your environment. export DEVKITARM=<path to>devkitARM")
endif

export TOPDIR ?= $(CURDIR)
include $(DEVKITARM)/3ds_rules

CTRPFLIB	?=	$(DEVKITPRO)/libctrpf

TARGET		:= 	udsrelay-shim
INCLUDES	:= 	Sources ../core/include
SOURCES 	:= 	Sources ../core/src
PSF 		:= 	udsrelay-shim.plgInfo

#---------------------------------------------------------------------------------
# options for code generation
#---------------------------------------------------------------------------------
ARCH	:=	-march=armv6k -mtune=mpcore -mfloat-abi=hard -mtp=soft

COMMONFLAGS	:=	-mword-relocations -ffunction-sections -fdata-sections -fno-strict-aliasing \
				-Wall -Wextra -O2 -fomit-frame-pointer -DNDEBUG=1 $(ARCH)

CFLAGS		:=	$(COMMONFLAGS) -std=gnu11 $(INCLUDE) -D__3DS__
CXXFLAGS	:=	$(COMMONFLAGS) -std=gnu++11 -fno-rtti -fno-exceptions $(INCLUDE) -D__3DS__

ASFLAGS		:=	$(ARCH)
LDFLAGS		:=	-T $(TOPDIR)/3gx.ld $(ARCH) -Os -Wl,--strip-discarded,--strip-debug,--gc-sections,--section-start,.text=0x07000100

LIBS		:=	-lctrpf -lctru -lm
LIBDIRS		:=	$(CTRPFLIB) $(CTRULIB) $(PORTLIBS)

#---------------------------------------------------------------------------------
ifneq ($(BUILD),$(notdir $(CURDIR)))
#---------------------------------------------------------------------------------
export BUILD	:=	build
export OUTPUT	:=	$(CURDIR)/$(TARGET)
export VPATH	:=	$(foreach dir,$(SOURCES),$(CURDIR)/$(dir))
export DEPSDIR	:=	$(CURDIR)/$(BUILD)

CFILES		:=	$(foreach dir,$(SOURCES),$(notdir $(wildcard $(dir)/*.c)))
CPPFILES	:=	$(foreach dir,$(SOURCES),$(notdir $(wildcard $(dir)/*.cpp)))
SFILES		:=	$(foreach dir,$(SOURCES),$(notdir $(wildcard $(dir)/*.s)))

export LD		:=	$(CXX)
export OFILES	:=	$(CPPFILES:.cpp=.o) $(CFILES:.c=.o) $(SFILES:.s=.o)
export INCLUDE	:=	$(foreach dir,$(INCLUDES),-I$(CURDIR)/$(dir)) \
					$(foreach dir,$(LIBDIRS),-I$(dir)/include) \
					-I$(CURDIR)/$(BUILD)
export LIBPATHS	:=	$(foreach dir,$(LIBDIRS),-L$(dir)/lib)

.PHONY: $(BUILD) clean all

all: $(BUILD)

$(BUILD):
	@[ -d $@ ] || mkdir -p $@
	@$(MAKE) --no-print-directory -C $(BUILD) -f $(CURDIR)/Makefile

clean:
	@echo clean ...
	@rm -fr $(BUILD) $(TARGET).elf $(TARGET).3gx

#---------------------------------------------------------------------------------
else
#---------------------------------------------------------------------------------

DEPENDS	:=	$(OFILES:.o=.d)

$(OUTPUT).3gx : $(OUTPUT).elf
$(OUTPUT).elf : $(OFILES)

%.3gx: %.elf
	@echo creating $(notdir $@)
	@3gxtool -s $^ $(TOPDIR)/$(PSF) $@

-include $(DEPENDS)

#---------------------------------------------------------------------------------
endif
#---------------------------------------------------------------------------------
